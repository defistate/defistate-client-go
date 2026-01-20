package arbitrum

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/defistate/defistate-client-go/chains"
	"github.com/defistate/defistate-client-go/engine"
	poolregistry "github.com/defistate/defistate-client-go/protocols/poolregistry"
	poolregistryindexer "github.com/defistate/defistate-client-go/protocols/poolregistry/indexer"
	tokenpoolregistry "github.com/defistate/defistate-client-go/protocols/tokenpoolregistry"
	tokenregistry "github.com/defistate/defistate-client-go/protocols/tokenregistry"
	tokenregistryindexer "github.com/defistate/defistate-client-go/protocols/tokenregistry/indexer"
	uniswapv2 "github.com/defistate/defistate-client-go/protocols/uniswapv2"
	uniswapv2indexer "github.com/defistate/defistate-client-go/protocols/uniswapv2/indexer"
	uniswapv3 "github.com/defistate/defistate-client-go/protocols/uniswapv3"
	uniswapv3indexer "github.com/defistate/defistate-client-go/protocols/uniswapv3/indexer"
	"github.com/stretchr/testify/assert"
)

// --- Mocks ---

// mockTransport simulates the low-level chains.Client
type mockTransport struct {
	stateCh chan *engine.State
	errCh   chan error
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		stateCh: make(chan *engine.State, 10),
		errCh:   make(chan error, 10),
	}
}
func (m *mockTransport) State() <-chan *engine.State { return m.stateCh }
func (m *mockTransport) Err() <-chan error           { return m.errCh }

// --- Indexer Mocks ---

type mockTokenIndexer struct{ called bool }

func (m *mockTokenIndexer) Index(tokens []tokenregistry.Token) tokenregistryindexer.IndexedTokenSystem {
	m.called = true
	return &mockIndexedTokenSystem{}
}

type mockPoolRegistryIndexer struct{ called bool }

func (m *mockPoolRegistryIndexer) Index(pr poolregistry.PoolRegistry) poolregistryindexer.IndexedPoolRegistry {
	m.called = true
	return &mockIndexedPoolRegistry{}
}

type mockUniswapV2Indexer struct{ called bool }

func (m *mockUniswapV2Indexer) Index(pools []uniswapv2.Pool) uniswapv2indexer.IndexedUniswapV2 {
	m.called = true
	return &mockIndexedUniswapV2{}
}

type mockUniswapV3Indexer struct{ called bool }

func (m *mockUniswapV3Indexer) Index(pools []uniswapv3.Pool) uniswapv3indexer.IndexedUniswapV3 {
	m.called = true
	return &mockIndexedUniswapV3{}
}

// --- Grapher Mock ---

type mockGrapher struct{ called bool }

func (m *mockGrapher) Graph(
	tp *tokenpoolregistry.TokenPoolRegistryView,
	tr tokenregistryindexer.IndexedTokenSystem,
	pr poolregistryindexer.IndexedPoolRegistry,
	v2 uniswapv2indexer.IndexedUniswapV2,
	v3 uniswapv3indexer.IndexedUniswapV3,
	resolver *chains.ProtocolResolver,
) (chains.TokenPoolGraph, error) {
	m.called = true
	return &mockTokenPoolGraph{}, nil
}

type mockIndexedTokenSystem struct {
	tokenregistryindexer.IndexedTokenSystem
}
type mockIndexedPoolRegistry struct {
	poolregistryindexer.IndexedPoolRegistry
}

// Helper to satisfy the ProtocolResolver requirement
func (m *mockIndexedPoolRegistry) GetByID(id uint64) (poolregistry.Pool, bool) {
	return poolregistry.Pool{}, false
}
func (m *mockIndexedPoolRegistry) GetProtocols() map[uint16]engine.ProtocolID {
	return map[uint16]engine.ProtocolID{}
}

type mockIndexedUniswapV2 struct {
	uniswapv2indexer.IndexedUniswapV2
}
type mockIndexedUniswapV3 struct {
	uniswapv3indexer.IndexedUniswapV3
}
type mockTokenPoolGraph struct{ chains.TokenPoolGraph }

// --- Test Suite ---

func TestClient_Lifecycle(t *testing.T) {
	// Setup Mocks
	transport := newMockTransport()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Create Client manually to bypass Dial's real network connection
	c := &Client{
		stream:              transport,
		logger:              logger,
		stateCh:             make(chan *State, 10),
		errCh:               make(chan error, 10),
		tokenIndexer:        &mockTokenIndexer{},
		poolRegistryIndexer: &mockPoolRegistryIndexer{},
		uniswapV2Indexer:    &mockUniswapV2Indexer{},
		uniswapV3Indexer:    &mockUniswapV3Indexer{},
		tokenPoolGrapher:    &mockGrapher{},
	}

	// Manual Context Setup (Simulating Dial)
	ctx, cancel := context.WithCancel(context.Background())
	c.ctx = ctx
	c.wg.Add(1)

	// Start Loop
	go c.loop()

	// 1. Send Data
	rawState := &engine.State{
		Block: engine.BlockSummary{Number: big.NewInt(100)},
		Protocols: map[engine.ProtocolID]engine.ProtocolState{
			"tokens":   {Schema: tokenregistry.Schema, Data: []tokenregistry.Token{}},
			"registry": {Schema: poolregistry.Schema, Data: poolregistry.PoolRegistry{}},
			"graph":    {Schema: tokenpoolregistry.Schema, Data: &tokenpoolregistry.TokenPoolRegistryView{}},
			"univ2":    {Schema: uniswapv2.Schema, Data: []uniswapv2.Pool{}},
			"univ3":    {Schema: uniswapv3.Schema, Data: []uniswapv3.Pool{}},
		},
	}

	transport.stateCh <- rawState

	// 2. Expect Result
	select {
	case processed := <-c.State():
		assert.Equal(t, int64(100), processed.Block.Number.Int64())
		assert.NotNil(t, processed.Graph)
		assert.NotNil(t, processed.ProtocolResolver)

		// Verify Mocks were called
		assert.True(t, c.tokenIndexer.(*mockTokenIndexer).called)
		assert.True(t, c.poolRegistryIndexer.(*mockPoolRegistryIndexer).called)
		assert.True(t, c.tokenPoolGrapher.(*mockGrapher).called)

	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for processed state")
	}

	// 3. Test Shutdown (Cancel Context)
	cancel()

	// Channels should be closed by the loop
	select {
	case _, ok := <-c.State():
		assert.False(t, ok, "State channel should be closed")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channels did not close")
	}
}

func TestClient_ErrorHandling(t *testing.T) {
	transport := newMockTransport()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	c := &Client{
		stream:  transport,
		logger:  logger,
		stateCh: make(chan *State, 1),
		errCh:   make(chan error, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.ctx = ctx
	c.wg.Add(1)
	go c.loop()
	defer cancel()

	// 1. Send Fatal Error
	expectedErr := fmt.Errorf("websocket disconnect")
	transport.errCh <- expectedErr

	select {
	case err := <-c.Err():
		assert.Equal(t, expectedErr, err)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for error")
	}
}

func TestClient_MissingDataValidation(t *testing.T) {
	transport := newMockTransport()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	c := &Client{
		stream:  transport,
		logger:  logger,
		stateCh: make(chan *State, 1),
		errCh:   make(chan error, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.ctx = ctx
	c.wg.Add(1)
	go c.loop()
	defer cancel()

	// 1. Send Incomplete Data (Missing Graph)
	incompleteState := &engine.State{
		Block: engine.BlockSummary{Number: big.NewInt(101)},
		Protocols: map[engine.ProtocolID]engine.ProtocolState{
			"tokens": {Schema: tokenregistry.Schema, Data: []tokenregistry.Token{}},
			// Missing other required protocols
		},
	}

	transport.stateCh <- incompleteState

	// 2. Ensure it didn't crash and didn't produce state
	select {
	case <-c.State():
		t.Fatal("Should not produce state for incomplete data")
	case <-time.After(100 * time.Millisecond):
		// This is expected behavior; the loop logs an error and continues
	}
}

func TestClient_Backpressure(t *testing.T) {
	// Test the "Warn-Then-Drop" behavior
	transport := newMockTransport()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Buffer size 0 to force immediate backpressure logic if no consumer
	c := &Client{
		stream:              transport,
		logger:              logger,
		stateCh:             make(chan *State), // Unbuffered!
		errCh:               make(chan error, 1),
		tokenIndexer:        &mockTokenIndexer{},
		poolRegistryIndexer: &mockPoolRegistryIndexer{},
		uniswapV2Indexer:    &mockUniswapV2Indexer{},
		uniswapV3Indexer:    &mockUniswapV3Indexer{},
		tokenPoolGrapher:    &mockGrapher{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.ctx = ctx
	c.wg.Add(1)
	go c.loop()
	defer cancel()

	rawState := &engine.State{
		Block: engine.BlockSummary{Number: big.NewInt(200)},
		Protocols: map[engine.ProtocolID]engine.ProtocolState{
			"tokens":   {Schema: tokenregistry.Schema, Data: []tokenregistry.Token{}},
			"registry": {Schema: poolregistry.Schema, Data: poolregistry.PoolRegistry{}},
			"graph":    {Schema: tokenpoolregistry.Schema, Data: &tokenpoolregistry.TokenPoolRegistryView{}},
		},
	}

	// 1. Send data while NO ONE is listening on c.State()
	transport.stateCh <- rawState

	// 2. Wait a moment. The loop should have tried to send, hit 'default', logged warning, and continued.
	time.Sleep(50 * time.Millisecond)

	// 3. Now start listening. We expect the PREVIOUS item might have been dropped
	// (depending on your loop implementation: default case = drop).
	// If you want to verify it was dropped:
	select {
	case <-c.State():
		t.Fatal("Expected state to be dropped due to backpressure")
	default:
		// Success: nothing in channel
	}
}

func TestOptions(t *testing.T) {
	// 1. Create specific mocks to verify assignment
	mockTokenIdx := &mockTokenIndexer{}
	mockPoolRegistryIdx := &mockPoolRegistryIndexer{}
	mockUniswapV2Idx := &mockUniswapV2Indexer{}
	mockUniswapV3Idx := &mockUniswapV3Indexer{}
	mockGrapher := &mockGrapher{}

	// 2. Initialize an empty client
	c := &Client{}

	// 3. Define the options to test
	opts := []Option{
		WithTokenIndexer(mockTokenIdx),
		WithPoolRegistryIndexer(mockPoolRegistryIdx),
		WithUniswapV2Indexer(mockUniswapV2Idx),
		WithUniswapV3Indexer(mockUniswapV3Idx),
		WithTokenPoolGrapher(mockGrapher),
	}

	// 4. Apply them manually (allowed since we are in package ethereum)
	for _, opt := range opts {
		opt.apply(c)
	}

	// 5. Assertions: Verify the internal fields point to our mocks
	// We compare pointers to ensure the exact object was assigned.
	assert.Same(t, mockTokenIdx, c.tokenIndexer, "WithTokenIndexer should set tokenIndexer")
	assert.Same(t, mockPoolRegistryIdx, c.poolRegistryIndexer, "WithPoolRegistryIndexer should set poolRegistryIndexer")
	assert.Same(t, mockUniswapV2Idx, c.uniswapV2Indexer, "WithUniswapV2Indexer should set uniswapV2Indexer")
	assert.Same(t, mockUniswapV3Idx, c.uniswapV3Indexer, "WithUniswapV3Indexer should set uniswapV3Indexer")
	assert.Same(t, mockGrapher, c.tokenPoolGrapher, "WithTokenPoolGrapher should set tokenPoolGrapher")
}
