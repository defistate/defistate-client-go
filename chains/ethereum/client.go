package ethereum

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/defistate/defistate-client-go/chains"
	"github.com/defistate/defistate-client-go/chains/ethereum/grapher"
	"github.com/defistate/defistate-client-go/engine"
	jsonrpcclient "github.com/defistate/defistate-client-go/streams/jsonrpc/client"
	ethstateops "github.com/defistate/defistate-client-go/streams/jsonrpc/stateops/chains/ethereum"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/defistate/defistate-client-go/protocols/poolregistry"
	poolregistryindexer "github.com/defistate/defistate-client-go/protocols/poolregistry/indexer"
	"github.com/defistate/defistate-client-go/protocols/tokenpoolregistry"
	tokenregistry "github.com/defistate/defistate-client-go/protocols/tokenregistry"
	tokenregistryindexer "github.com/defistate/defistate-client-go/protocols/tokenregistry/indexer"
	"github.com/defistate/defistate-client-go/protocols/uniswapv2"
	uniswapv2indexer "github.com/defistate/defistate-client-go/protocols/uniswapv2/indexer"
	"github.com/defistate/defistate-client-go/protocols/uniswapv3"
	uniswapv3indexer "github.com/defistate/defistate-client-go/protocols/uniswapv3/indexer"
)

// Client orchestrates the ingestion and processing of DeFi state.
// Its lifecycle is bound to the context passed during Dial.
type Client struct {
	stream  chains.Client
	logger  chains.Logger
	stateCh chan *State
	errCh   chan error

	// Immutable Indexers (set via Options during Dial)
	tokenPoolGrapher    chains.TokenPoolGrapher
	tokenIndexer        chains.TokenIndexer
	poolRegistryIndexer chains.PoolRegistryIndexer
	uniswapV2Indexer    chains.UniswapV2Indexer
	uniswapV3Indexer    chains.UniswapV3Indexer

	ctx context.Context
	wg  sync.WaitGroup
}

// Option configures the Client.
// The interface method is unexported to prevent external modification after Dial.
type Option interface {
	apply(*Client)
}

type funcOption func(*Client)

func (f funcOption) apply(p *Client) {
	f(p)
}

func newOption(f func(*Client)) Option {
	return funcOption(f)
}

// Dial establishes the connection and starts the processing loop.
// The returned Client will remain active until the provided ctx is cancelled.
func Dial(
	ctx context.Context,
	url string,
	logger chains.Logger,
	prometheusRegistry prometheus.Registerer,
	opts ...Option,
) (*Client, error) {

	stateOps, err := ethstateops.NewStateOps(
		logger,
		prometheusRegistry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create state ops: %w", err)
	}

	clientCfg := jsonrpcclient.Config{
		URL:              url,
		Logger:           logger,
		BufferSize:       1,
		StatePatcher:     stateOps.Patch,
		StateDecoder:     stateOps.DecodeStateJSON,
		StateDiffDecoder: stateOps.DecodeStateDiffJSON,
	}

	client, err := jsonrpcclient.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to dial defistate stream url: %w", err)
	}

	tokenPoolGrapher, err := grapher.NewGrapher()
	if err != nil {
		return nil, fmt.Errorf("failed to create grapher: %w", err)
	}

	p := &Client{
		stream:              client,
		logger:              logger,
		stateCh:             make(chan *State, 1),
		errCh:               make(chan error, 1),
		tokenIndexer:        tokenregistryindexer.New(),
		poolRegistryIndexer: poolregistryindexer.New(),
		tokenPoolGrapher:    tokenPoolGrapher,
		uniswapV2Indexer:    uniswapv2indexer.New(),
		uniswapV3Indexer:    uniswapv3indexer.New(),
	}

	for _, opt := range opts {
		opt.apply(p)
	}

	// Bind the Client's lifecycle to the user-provided context
	p.ctx = ctx
	p.wg.Add(1)
	go p.loop()

	p.logger.Info("Client started", "url", url)
	return p, nil
}

// State channel is best-effort; if consumer is slow, updates may be dropped
func (p *Client) State() <-chan *State {
	return p.stateCh
}

func (p *Client) Err() <-chan error {
	return p.errCh
}

func (p *Client) loop() {
	defer p.wg.Done()
	defer func() {
		close(p.stateCh)
		close(p.errCh)
		p.logger.Info("Client stopped")
	}()

	for {
		select {
		case <-p.ctx.Done():
			return

		case err := <-p.stream.Err():
			p.logger.Error("Fatal client error", "err", err)
			select {
			case p.errCh <- err:
			case <-p.ctx.Done():
			}
			return

		case rawState, ok := <-p.stream.State():
			if !ok {
				p.logger.Error("Upstream state channel closed")
				return
			}

			processed, err := p.processState(rawState)
			if err != nil {
				p.logger.Error("Failed to process state", "block", rawState.Block.Number, "err", err)
				continue
			}

			select {
			case p.stateCh <- processed:
			case <-p.ctx.Done():
				return
			default:
				p.logger.Warn("State buffer full, discarding processed state...", "block", rawState.Block.Number)
			}
		}
	}
}

type State struct {
	Graph               chains.TokenPoolGraph
	IndexedTokenSystem  tokenregistryindexer.IndexedTokenSystem
	IndexedPoolRegistry poolregistryindexer.IndexedPoolRegistry
	IndexedUniswapV2    uniswapv2indexer.IndexedUniswapV2
	IndexedUniswapV3    uniswapv3indexer.IndexedUniswapV3
	ProtocolResolver    *chains.ProtocolResolver
	Block               engine.BlockSummary
	ProcessedAtUnixNs   uint64
}

func (p *Client) processState(rawState *engine.State) (*State, error) {

	indexingStart := time.Now()
	p.logger.Info("New state received, starting processing", "block", rawState.Block.Number)

	var wg sync.WaitGroup
	wg.Add(4)

	var (
		rawGraph         *tokenpoolregistry.TokenPoolRegistryView
		tokenData        []tokenregistry.Token
		poolRegistryData *poolregistry.PoolRegistry

		allUniswapV2Data []uniswapv2.Pool
		allUniswapV3Data []uniswapv3.Pool

		indexedTokenSystem  tokenregistryindexer.IndexedTokenSystem
		indexedPoolRegistry poolregistryindexer.IndexedPoolRegistry
		indexedUniswapV2    uniswapv2indexer.IndexedUniswapV2
		indexedUniswapV3    uniswapv3indexer.IndexedUniswapV3
	)

	// first, get all data with switch on Protocol.Schema
	for _, protocol := range rawState.Protocols {
		switch protocol.Schema {
		case tokenregistry.Schema:
			if tokenData != nil {
				return nil, fmt.Errorf("multiple token protocol data found")
			}
			tokenData = protocol.Data.([]tokenregistry.Token)
		case poolregistry.Schema:
			if poolRegistryData != nil {
				return nil, fmt.Errorf("multiple pool registry protocol data found")
			}
			d := protocol.Data.(poolregistry.PoolRegistry)
			poolRegistryData = &d

		case tokenpoolregistry.Schema:
			if rawGraph != nil {
				return nil, fmt.Errorf("multiple graph data found")
			}
			rawGraph = protocol.Data.(*tokenpoolregistry.TokenPoolRegistryView)
		case uniswapv2.Schema:
			allUniswapV2Data = append(allUniswapV2Data, protocol.Data.([]uniswapv2.Pool)...)
		case uniswapv3.Schema:
			allUniswapV3Data = append(allUniswapV3Data, protocol.Data.([]uniswapv3.Pool)...)
		}
	}

	if rawGraph == nil {
		return nil, fmt.Errorf("No token pool graph data found in raw state. Block %d", rawState.Block.Number)
	}
	if tokenData == nil {
		return nil, fmt.Errorf("No token system data found in raw state. Block %d", rawState.Block.Number)
	}

	if poolRegistryData == nil {
		return nil, fmt.Errorf("No pool registry data found in raw state. Block %d", rawState.Block.Number)
	}

	go func() {
		defer wg.Done()
		indexedTokenSystem = p.tokenIndexer.Index(tokenData)
	}()

	go func() {
		defer wg.Done()
		indexedPoolRegistry = p.poolRegistryIndexer.Index(*poolRegistryData)
	}()

	go func() {
		defer wg.Done()
		indexedUniswapV2 = p.uniswapV2Indexer.Index(allUniswapV2Data)
	}()
	go func() {
		defer wg.Done()
		indexedUniswapV3 = p.uniswapV3Indexer.Index(allUniswapV3Data)
	}()

	wg.Wait()

	// log metrics
	indexingDuration := time.Since(indexingStart)
	p.logger.Info("All known protocols indexed in parallel", "block", rawState.Block.Number, "duration_ms", indexingDuration.Milliseconds())

	graphingStart := time.Now()
	protocolIDToProtocolSchema := map[engine.ProtocolID]engine.ProtocolSchema{}
	for id, protocol := range rawState.Protocols {
		protocolIDToProtocolSchema[id] = protocol.Schema
	}

	protocolResolver := chains.NewProtocolResolver(
		protocolIDToProtocolSchema,
		indexedPoolRegistry,
	)

	graph, err := p.tokenPoolGrapher.Graph(
		rawGraph,
		indexedPoolRegistry,
		indexedUniswapV2,
		indexedUniswapV3,
		protocolResolver,
	)

	if err != nil {
		return nil, fmt.Errorf("Grapher error %v", err)
	}

	graphingDuration := time.Since(graphingStart)
	p.logger.Info("Analytical graph built", "block", rawState.Block.Number, "duration_ms", graphingDuration.Milliseconds())

	state := &State{
		Graph:               graph,
		IndexedTokenSystem:  indexedTokenSystem,
		IndexedPoolRegistry: indexedPoolRegistry,
		IndexedUniswapV2:    indexedUniswapV2,
		IndexedUniswapV3:    indexedUniswapV3,
		ProtocolResolver:    protocolResolver,
		Block:               rawState.Block,
		ProcessedAtUnixNs:   uint64(time.Now().UnixNano()),
	}

	return state, nil

}

// Options Constructors for the Client

func WithTokenIndexer(indexer chains.TokenIndexer) Option {
	return newOption(func(p *Client) {
		p.tokenIndexer = indexer
	})
}

func WithPoolRegistryIndexer(indexer chains.PoolRegistryIndexer) Option {
	return newOption(func(p *Client) {
		p.poolRegistryIndexer = indexer
	})
}

func WithUniswapV2Indexer(indexer chains.UniswapV2Indexer) Option {
	return newOption(func(p *Client) {
		p.uniswapV2Indexer = indexer
	})
}

func WithUniswapV3Indexer(indexer chains.UniswapV3Indexer) Option {
	return newOption(func(p *Client) {
		p.uniswapV3Indexer = indexer
	})
}

func WithTokenPoolGrapher(grapher chains.TokenPoolGrapher) Option {
	return newOption(func(p *Client) {
		p.tokenPoolGrapher = grapher
	})
}
