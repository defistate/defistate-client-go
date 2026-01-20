package graph

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/defistate/defistate-client-go/bitset"
	"github.com/defistate/defistate-client-go/engine"
	poolregistry "github.com/defistate/defistate-client-go/protocols/poolregistry"
	tokenpoolregistry "github.com/defistate/defistate-client-go/protocols/tokenpoolregistry"
	tokenregistry "github.com/defistate/defistate-client-go/protocols/tokenregistry"

	poolregistryindexer "github.com/defistate/defistate-client-go/protocols/poolregistry/indexer"
	tokenindexer "github.com/defistate/defistate-client-go/protocols/tokenregistry/indexer"
	uniswapv2 "github.com/defistate/defistate-client-go/protocols/uniswapv2"
	uniswapv2calculator "github.com/defistate/defistate-client-go/protocols/uniswapv2/calculator"
	uniswapv2indexer "github.com/defistate/defistate-client-go/protocols/uniswapv2/indexer"
	uniswapv3 "github.com/defistate/defistate-client-go/protocols/uniswapv3"
	uniswapv3calculator "github.com/defistate/defistate-client-go/protocols/uniswapv3/calculator"
	uniswapv3indexer "github.com/defistate/defistate-client-go/protocols/uniswapv3/indexer"
	"github.com/ethereum/go-ethereum/common"
)

type TokenPoolPath struct {
	TokenInID  uint64
	TokenOutID uint64
	PoolID     uint64
}

// IndexedPoolRegistry defines the methods for accessing indexed pool registry data.
type IndexedPoolRegistry interface {
	GetByID(id uint64) (poolregistry.Pool, bool)
	GetByAddress(address common.Address) (poolregistry.Pool, bool)
	All() []poolregistry.Pool
}

// bigIntPool is a package-level pool for reusing *big.Int objects.
var bigIntPool = sync.Pool{
	New: func() any {
		return new(big.Int)
	},
}

// GetAmountOutFunc is for high-fidelity quoting using big.Int.
type GetAmountOutFunc func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error)

// findSwapPathsState encapsulates the state required for the Bellman-Ford-like
// swap path finding algorithm.
type findSwapPathsState struct {
	start   int
	current int
	end     int
	paths   [][]TokenPoolPath // vertex index -> path
	costs   []*big.Int        // vertex index -> cost
	known   []bitset.BitSet   // vertex index -> vertex index
	temp    *big.Int
}

// Graph is a reusable, stateless algorithmic engine for a single state snapshot.
type Graph struct {
	// Raw data views required for lookups.
	tokenPool *tokenpoolregistry.TokenPoolRegistryView

	// Internal lookup maps for fast access.
	tokenToIndex map[uint64]int
	poolToIndex  map[uint64]int

	allGetAmountOutFuncs []GetAmountOutFunc
}

// NewGraph creates a new Graph instance. It pre-processes the raw view data
// by building lookup maps and the two distinct slices of computation functions.
func NewGraph(
	tokenPool *tokenpoolregistry.TokenPoolRegistryView,
	tokenRegistryView []tokenregistry.Token,
	poolRegistryView poolregistry.PoolRegistry,
	protocols map[engine.ProtocolID]engine.ProtocolState,
) (*Graph, error) {
	tokenToIndex := make(map[uint64]int, len(tokenPool.Tokens))
	for i, id := range tokenPool.Tokens {
		tokenToIndex[id] = i
	}

	poolToIndex := make(map[uint64]int, len(tokenPool.Pools))
	for i, id := range tokenPool.Pools {
		poolToIndex[id] = i
	}

	allGetAmountOutFuncs := make([]GetAmountOutFunc, len(tokenPool.Pools))
	indexedPoolRegistry := poolregistryindexer.NewIndexablePoolRegistry(poolRegistryView)
	indexedTokenRegistry := tokenindexer.NewIndexableTokenSystem(tokenRegistryView)

	protocolIDToIndexed := make(map[engine.ProtocolID]any)

	for i, poolID := range tokenPool.Pools {
		poolInfo, ok := indexedPoolRegistry.GetByID(poolID)
		if !ok {
			continue
		}

		// get the protocol for the pool
		protocolId := poolRegistryView.Protocols[poolInfo.Protocol]
		protocol, ok := protocols[protocolId]

		if !ok {
			return nil, fmt.Errorf("protocol with ProtocolID %s not found", protocolId)
		}

		// find the pool data from schema

		switch protocol.Schema {
		case uniswapv2.Schema:
			var (
				indexedProtocol any
				ok              bool
			)
			indexedProtocol, ok = protocolIDToIndexed[protocolId]
			if !ok {
				// create the indexed protocol
				indexedProtocol = uniswapv2indexer.NewIndexableUniswapV2System(protocol.Data.([]uniswapv2.Pool))
				protocolIDToIndexed[protocolId] = indexedProtocol
			}

			// get pool from protocol
			pool, found := indexedProtocol.(*uniswapv2indexer.IndexableUniswapV2System).GetByID(poolID)
			if !found {
				continue // maybe panic?
			}

			// Check for Fee-On-Transfer tokens
			t0, ok0 := indexedTokenRegistry.GetByID(pool.Token0)
			t1, ok1 := indexedTokenRegistry.GetByID(pool.Token1)
			if (ok0 && t0.FeeOnTransferPercent > 0) || (ok1 && t1.FeeOnTransferPercent > 0) {
				// Fee-on-transfer tokens break standard amount out calculations.
				// We leave the calculator as nil to ignore this pool in routing.
				continue
			}

			// Build the precise function using the live calculator.
			allGetAmountOutFuncs[i] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
				return uniswapv2calculator.GetAmountOut(amountIn, tokenInID, tokenOutID, pool)
			}
		case uniswapv3.Schema:
			var (
				indexedProtocol any
				ok              bool
			)
			indexedProtocol, ok = protocolIDToIndexed[protocolId]
			if !ok {
				// create the indexed protocol
				indexedProtocol = uniswapv3indexer.NewIndexableUniswapV3System(protocol.Data.([]uniswapv3.Pool))
				protocolIDToIndexed[protocolId] = indexedProtocol
			}

			// get pool from protocol
			pool, found := indexedProtocol.(*uniswapv3indexer.IndexableUniswapV3System).GetByID(poolID)
			if !found {
				continue // maybe panic?
			}

			// Check for Fee-On-Transfer tokens
			t0, ok0 := indexedTokenRegistry.GetByID(pool.Token0)
			t1, ok1 := indexedTokenRegistry.GetByID(pool.Token1)
			if (ok0 && t0.FeeOnTransferPercent > 0) || (ok1 && t1.FeeOnTransferPercent > 0) {
				// Fee-on-transfer tokens break standard amount out calculations.
				// We leave the calculator as nil to ignore this pool in routing.
				continue
			}

			// Build the precise function using the live calculator.
			allGetAmountOutFuncs[i] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
				return uniswapv3calculator.GetAmountOut(amountIn, nil, tokenInID, pool)
			}
		}
	}

	return &Graph{
		tokenPool:            tokenPool,
		tokenToIndex:         tokenToIndex,
		poolToIndex:          poolToIndex,
		allGetAmountOutFuncs: allGetAmountOutFuncs,
	}, nil
}

// FindBestSwapPath searches the graph for the most profitable swap path between two tokens.
// It uses a "copy-and-patch" strategy to handle state overrides.
func (g *Graph) FindBestSwapPath(
	tokenInID uint64,
	tokenOutID uint64,
	amountIn *big.Int,
	runs int,
) ([]TokenPoolPath, *big.Int, error) {

	// --- Step 1: Create a temporary, patched slice of swap functions ---
	getAmountOutFuncs := make([]GetAmountOutFunc, len(g.allGetAmountOutFuncs))
	copy(getAmountOutFuncs, g.allGetAmountOutFuncs)

	// --- Step 2: Initialize and run the pathfinding algorithm ---
	startIndex, exists := g.tokenToIndex[tokenInID]
	if !exists {
		return nil, nil, fmt.Errorf("start tokenregistry %d not found in the graph", tokenInID)
	}

	endIndex, exists := g.tokenToIndex[tokenOutID]
	if !exists {
		return nil, nil, fmt.Errorf("end tokenregistry %d not found in the graph", tokenOutID)
	}

	numTokens := len(g.tokenPool.Tokens)
	state := &findSwapPathsState{
		start: startIndex,
		end:   endIndex,
		paths: make([][]TokenPoolPath, numTokens),
		costs: make([]*big.Int, numTokens),
		known: make([]bitset.BitSet, numTokens),
		temp:  bigIntPool.Get().(*big.Int).SetUint64(0),
	}

	// This defer block is CRITICAL. It ensures all rented objects are returned.
	defer func() {
		// Return the scratchpad int
		bigIntPool.Put(state.temp.SetUint64(0))
		// Return all integers used in the costs slice
		for _, cost := range state.costs {
			if cost != nil {
				bigIntPool.Put(cost.SetUint64(0))
			}
		}
	}()

	for i := 0; i < numTokens; i++ {
		state.known[i] = bitset.NewBitSet(uint64(numTokens))
		// Rent *big.Int objects from the pool instead of allocating new ones
		state.costs[i] = bigIntPool.Get().(*big.Int).SetUint64(0)

	}

	state.costs[startIndex].Set(amountIn)

	for i := 0; i < runs; i++ {
		for j := 0; j < numTokens; j++ {
			if state.costs[j].Sign() == 0 {
				continue
			}
			state.current = j
			if err := g.findSwapPath(state, getAmountOutFuncs); err != nil {
				return nil, nil, err
			}
		}
	}

	// --- Step 3: Reconstruct and return the best path found ---
	bestPath := state.paths[endIndex]
	if bestPath == nil {
		return nil, nil, nil // No path found between the two tokens.
	}

	return bestPath, new(big.Int).Set(state.costs[endIndex]), nil
}

// findSwapPath is the core Bellman-Ford-like relaxation step for finding the best swap paths.
func (g *Graph) findSwapPath(state *findSwapPathsState, getAmountOutFuncs []GetAmountOutFunc) error {
	currentIndex := state.current
	currentCost := state.costs[currentIndex]
	currentKnown := state.known[currentIndex]
	currentPath := state.paths[currentIndex]
	currentTokenID := g.tokenPool.Tokens[currentIndex]

	if currentKnown.IsSet(uint64(currentIndex)) {
		return errors.New("cycle detected in path history")
	}

	maxAmountOut := state.temp
	for _, edgeIndex := range g.tokenPool.Adjacency[currentIndex] {
		targetIndex := g.tokenPool.EdgeTargets[edgeIndex]

		if currentKnown.IsSet(uint64(targetIndex)) {
			continue
		}

		targetTokenID := g.tokenPool.Tokens[targetIndex]
		bestPoolIndex := -1
		maxAmountOut.SetUint64(0)
		for _, poolIndex := range g.tokenPool.EdgePools[edgeIndex] {
			getAmountOut := getAmountOutFuncs[poolIndex]
			if getAmountOut == nil {
				continue
			}

			amountOut, err := getAmountOut(currentCost, currentTokenID, targetTokenID)
			if err == nil && amountOut.Cmp(maxAmountOut) == 1 {
				maxAmountOut.Set(amountOut)
				bestPoolIndex = poolIndex
			}
		}

		if bestPoolIndex == -1 {
			continue

		}
		if maxAmountOut.Cmp(state.costs[targetIndex]) == 1 {
			state.costs[targetIndex].Set(maxAmountOut)
			poolID := g.tokenPool.Pools[bestPoolIndex]
			newPath := make([]TokenPoolPath, len(currentPath)+1)
			copy(newPath, currentPath)
			newPath[len(currentPath)] = TokenPoolPath{
				TokenInID:  currentTokenID,
				TokenOutID: targetTokenID,
				PoolID:     poolID,
			}
			state.paths[targetIndex] = newPath
			state.known[targetIndex].SetFrom(currentKnown)
			state.known[targetIndex].Set(uint64(currentIndex))
		}
	}
	return nil
}
