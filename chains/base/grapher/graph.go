package grapher

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/defistate/defistate-client-go/bitset"
	"github.com/defistate/defistate-client-go/chains"

	tokenpoolregistry "github.com/defistate/defistate-client-go/protocols/tokenpoolregistry"
	uniswapv2 "github.com/defistate/defistate-client-go/protocols/uniswapv2"
	uniswapv2calculator "github.com/defistate/defistate-client-go/protocols/uniswapv2/calculator"
	uniswapv3 "github.com/defistate/defistate-client-go/protocols/uniswapv3"
	uniswapv3calculator "github.com/defistate/defistate-client-go/protocols/uniswapv3/calculator"

	poolregistryindexer "github.com/defistate/defistate-client-go/protocols/poolregistry/indexer"
	uniswapv2indexer "github.com/defistate/defistate-client-go/protocols/uniswapv2/indexer"
	uniswapv3indexer "github.com/defistate/defistate-client-go/protocols/uniswapv3/indexer"
)

/* Notes
*
* 1. The output of all the Graph methods depend on the correctness of the calculator packages for the implemented protocols.
* 2. The output of GetExchangeRates depends on the input. Ensure a sufficiently high amount input amount is used to ensure all desired tokens have an exchange rate.
* 3. FindArbitrageCycles (and variants) use the king-of-the-hill-algorithm. To find all possible cycles, callers need to call the function(s) multiple times to ensure all paths are discovered.
* 4. We switched from all paths detection for  FindArbitrageCycles (and variants)  to king-of-the-hill because of the amount of wasted computations that happened downstream (a single chosen cycle invalidates the others, so what's the point).
* 5. Using pools for big.Int and big.Float improve performance (see benchmarks)
 */

// bigIntPool is a package-level pool for reusing *big.Int objects.
var bigIntPool = sync.Pool{
	New: func() any {
		return new(big.Int)
	},
}

var bigFloatPool = sync.Pool{
	New: func() any {
		return new(big.Float)
	},
}

// GetAmountOutFunc is for high-fidelity quoting using big.Int.
type GetAmountOutFunc func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error)

// GetAmountOutFromCacheFunc calculates amountOut using a cache
type GetAmountOutFromCacheFunc func(amountIn float64, tokenInID, tokenOutID uint64) (float64, error)

// GetReservesFunc is for high-fidelity reserve lookups using big.Int.
type GetReservesFunc func(tokenInID, tokenOutID uint64) (reserveIn, reserveOut *big.Int, err error)

// Graph is a reusable, stateless algorithmic engine for a single state snapshot.
type Graph struct {
	// Raw data views required for lookups.
	rawGraph            *tokenpoolregistry.TokenPoolRegistryView
	indexedPoolRegistry poolregistryindexer.IndexedPoolRegistry
	indexedUniswapV2    uniswapv2indexer.IndexedUniswapV2
	indexedUniswapV3    uniswapv3indexer.IndexedUniswapV3

	// Internal lookup maps for fast access.
	tokenToIndex     map[uint64]int
	poolToIndex      map[uint64]int
	protocolResolver *chains.ProtocolResolver
	// Pre-computed slices of functions for different use cases.
	allGetAmountOutFuncs    []GetAmountOutFunc
	getReservesFuncs        []GetReservesFunc
	activeGetAmountOutFuncs []GetAmountOutFunc
}

// NewGraph creates a new Graph instance. It pre-processes the raw view data
// by building lookup maps and the two distinct slices of computation functions.
func NewGraph(
	rawGraph *tokenpoolregistry.TokenPoolRegistryView,
	indexedPoolRegistry poolregistryindexer.IndexedPoolRegistry,
	indexedUniswapV2 uniswapv2indexer.IndexedUniswapV2,
	indexedUniswapV3 uniswapv3indexer.IndexedUniswapV3,
	activePools map[uint64]struct{},
	protocolResolver *chains.ProtocolResolver,
) (*Graph, error) {

	tokenToIndex := make(map[uint64]int, len(rawGraph.Tokens))
	for i, id := range rawGraph.Tokens {
		tokenToIndex[id] = i
	}

	poolToIndex := make(map[uint64]int, len(rawGraph.Pools))
	for i, id := range rawGraph.Pools {
		poolToIndex[id] = i
	}

	// --- Pre-computation of Function Slices ---
	allGetAmountOutFuncs := make([]GetAmountOutFunc, len(rawGraph.Pools))
	getReservesFuncs := make([]GetReservesFunc, len(rawGraph.Pools))
	activeGetAmountOutFuncs := make([]GetAmountOutFunc, len(rawGraph.Pools))

	for i, poolID := range rawGraph.Pools {
		poolInfo, ok := indexedPoolRegistry.GetByID(poolID)
		if !ok {
			continue
		}

		schema, ok := protocolResolver.ResolveSchemaFromPoolID(poolInfo.ID)
		if !ok {
			continue
		}
		switch schema {
		case uniswapv2.Schema:
			pool, found := indexedUniswapV2.GetByID(poolID)
			if !found {
				continue // maybe panic?
			}

			// Build the precise function using the live calculator.
			allGetAmountOutFuncs[i] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
				return uniswapv2calculator.GetAmountOut(amountIn, tokenInID, tokenOutID, pool)
			}
			// Build the reserves function.
			getReservesFuncs[i] = func(tokenInID, tokenOutID uint64) (*big.Int, *big.Int, error) {
				return uniswapv2calculator.GetReserves(tokenInID, tokenOutID, pool)
			}
			// Build the cached function if this pool is in the active set.
			if _, ok := activePools[poolID]; ok {
				activeGetAmountOutFuncs[i] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
					return uniswapv2calculator.GetAmountOut(amountIn, tokenInID, tokenOutID, pool)
				}
			}

		case uniswapv3.Schema:
			pool, found := indexedUniswapV3.GetByID(poolID)
			if !found {
				continue // maybe panic?
			}
			allGetAmountOutFuncs[i] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
				return uniswapv3calculator.GetAmountOut(amountIn, nil, tokenInID, pool)
			}
			getReservesFuncs[i] = func(tokenInID, tokenOutID uint64) (*big.Int, *big.Int, error) {
				reserveTokenOut, err := uniswapv3calculator.GetAmountOut(uniswapv3calculator.MaxUint256, nil, tokenInID, pool)
				if err != nil {
					return nil, nil, err
				}

				reserveTokenIn, err := uniswapv3calculator.GetAmountOut(uniswapv3calculator.MaxUint256, nil, tokenOutID, pool)
				if err != nil {
					return nil, nil, err
				}
				return reserveTokenIn, reserveTokenOut, nil
			}
			if _, ok := activePools[poolID]; ok {
				activeGetAmountOutFuncs[i] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
					return uniswapv3calculator.GetAmountOut(amountIn, nil, tokenInID, pool)
				}
			}
		}
	}

	return &Graph{
		rawGraph:                rawGraph,
		indexedPoolRegistry:     indexedPoolRegistry,
		indexedUniswapV2:        indexedUniswapV2,
		indexedUniswapV3:        indexedUniswapV3,
		tokenToIndex:            tokenToIndex,
		poolToIndex:             poolToIndex,
		protocolResolver:        protocolResolver,
		allGetAmountOutFuncs:    allGetAmountOutFuncs,
		activeGetAmountOutFuncs: activeGetAmountOutFuncs,
		getReservesFuncs:        getReservesFuncs,
	}, nil

}

func (g *Graph) Raw() *tokenpoolregistry.TokenPoolRegistryView {
	// clone?
	return g.rawGraph
}

// GetPoolsForToken finds all pools connected to a given token by traversing the adjacency graph.
func (g *Graph) GetPoolsForToken(tokenID uint64) ([]uint64, error) {
	tokenIndex, exists := g.tokenToIndex[tokenID]
	if !exists {
		return nil, nil
	}
	edgeIndices := g.rawGraph.Adjacency[tokenIndex]
	if len(edgeIndices) == 0 {
		return nil, nil
	}
	uniquePoolIDs := make(map[uint64]struct{})
	for _, edgeIndex := range edgeIndices {
		poolIndices := g.rawGraph.EdgePools[edgeIndex]
		for _, poolIndex := range poolIndices {
			poolID := g.rawGraph.Pools[poolIndex]
			uniquePoolIDs[poolID] = struct{}{}
		}
	}
	result := make([]uint64, 0, len(uniquePoolIDs))
	for id := range uniquePoolIDs {
		result = append(result, id)
	}
	return result, nil
}

// GetTokensForPool finds the token IDs associated with a specific pool ID.
// It leverages the various indexed views to perform this reverse lookup efficiently.
func (g *Graph) GetTokensForPool(poolID uint64) ([]uint64, error) {
	poolInfo, ok := g.indexedPoolRegistry.GetByID(poolID)
	if !ok {
		return nil, fmt.Errorf("pool not found with ID %d", poolID)
	}
	schema, ok := g.protocolResolver.ResolveSchemaFromPoolID(poolInfo.ID)
	if !ok {
		return nil, fmt.Errorf("protocol schema not found for pool ID %d", poolID)
	}
	switch schema {
	case uniswapv2.Schema:
		pool, found := g.indexedUniswapV2.GetByID(poolID)
		if found {
			return []uint64{pool.Token0, pool.Token1}, nil
		}
	case uniswapv3.Schema:
		pool, found := g.indexedUniswapV3.GetByID(poolID)
		if found {
			return []uint64{pool.Token0, pool.Token1}, nil
		}
	}

	return nil, nil
}

// findConversionPathState encapsulates the state required for the Bellman-Ford-like
// pathfinding algorithm used in GetExchangeRates.
type findConversionPathState struct {
	start                    int                      // starting vertex index
	current                  int                      // current vertex index being processed
	paths                    [][]chains.TokenPoolPath // vertex index -> path to this token
	costs                    []*big.Int               // vertex index -> cost
	reserves                 []*big.Int               // vertex index -> reserve
	known                    []bitset.BitSet          // vertex index -> vertex index
	bestConnection           []int                    // edge index -> pool index
	bestConnectionComputed   bitset.BitSet            // edge index -> whether the best connection has been computed
	reserveForBestConnection []*big.Int               // edge index -> reserve for the best connection
	temp                     *big.Int
}

// GetExchangeRates calculates the equivalent value of a given amount of a base token
// across all other tokens in the graph using a Bellman-Ford-like algorithm.
// It can be constrained to only propagate prices from a specific set of allowed source tokens.
func (g *Graph) GetExchangeRates(
	baseAmountIn *big.Int,
	baseTokenID uint64,
	runs int,
	allowedSourceTokens map[uint64]struct{}, // New parameter
) (map[uint64]*big.Int, error) {

	// Step 1: Find the internal index for the starting token.
	baseIndex, exists := g.tokenToIndex[baseTokenID]
	if !exists {
		return nil, fmt.Errorf("token %d not found in the graph", baseTokenID)
	}

	// Step 2: Initialize the state for the pathfinding search.
	numTokens := len(g.rawGraph.Tokens)
	numEdges := len(g.rawGraph.EdgePools)

	state := &findConversionPathState{
		start:                    baseIndex,
		paths:                    make([][]chains.TokenPoolPath, numTokens),
		costs:                    make([]*big.Int, numTokens),
		known:                    make([]bitset.BitSet, numTokens),
		bestConnection:           make([]int, numEdges),
		bestConnectionComputed:   bitset.NewBitSet(uint64(numEdges)),
		reserveForBestConnection: make([]*big.Int, numEdges),
		reserves:                 make([]*big.Int, numTokens),
		temp:                     bigIntPool.Get().(*big.Int).SetUint64(0), // Get from pool
	}

	// This defer block ensures all temporary, pooled objects are returned.
	defer func() {
		bigIntPool.Put(state.temp.SetUint64(0))
		for _, r := range state.reserves {
			if r != nil {
				bigIntPool.Put(r.SetUint64(0))
			}
		}
		for _, r := range state.reserveForBestConnection {
			if r != nil {
				bigIntPool.Put(r.SetUint64(0))
			}
		}
	}()

	for i := range numTokens {
		state.known[i] = bitset.NewBitSet(uint64(numTokens))
		// Rent from pool for temporary state
		state.reserves[i] = bigIntPool.Get().(*big.Int).SetUint64(0) // ensure zero value
		// Allocate new for returned data
		state.costs[i] = new(big.Int)
	}
	state.costs[baseIndex].Set(baseAmountIn)
	for i := range numEdges {
		state.bestConnection[i] = -1 // -1 indicates no best connection yet
		// Rent from pool for temporary state
		state.reserveForBestConnection[i] = bigIntPool.Get().(*big.Int).SetUint64(0) // ensure zero value
	}

	// Step 3: Iteratively "relax" the edges for a set number of runs.
	for i := 0; i < runs; i++ {
		for j := 0; j < numTokens; j++ {
			if state.costs[j].Sign() == 0 {
				continue // Skip tokens that haven't been reached yet.
			}

			// Convert the internal index to the external token ID.
			currentTokenID := g.rawGraph.Tokens[j]
			// If a set of allowed source tokens is provided, check if the current
			// token is in that set before allowing it to propagate its price.
			if allowedSourceTokens != nil {
				if _, isAllowed := allowedSourceTokens[currentTokenID]; !isAllowed {
					continue // This token is not allowed to be a source.
				}
			}

			state.current = j
			if err := g.getExchangeRatesUsingMaxReservePath(state); err != nil {
				return nil, err
			}
		}
	}

	// Step 4: Convert the final costs slice back to a map for the user.
	finalExchangeRates := make(map[uint64]*big.Int, len(state.costs))
	for i, cost := range state.costs {
		if cost.Sign() != 0 {
			tokenID := g.rawGraph.Tokens[i]
			finalExchangeRates[tokenID] = cost
		}
	}

	// ensure baseToken equivalent equal to baseAmountIn
	finalExchangeRates[baseTokenID] = new(big.Int).Set(baseAmountIn)
	return finalExchangeRates, nil
}

// getExchangeRatesUsingMaxReservePath is the core of the algorithm. It uses the
// pre-computed swap functions for maximum performance.
// it sets connections based on maxReserve
func (g *Graph) getExchangeRatesUsingMaxReservePath(
	state *findConversionPathState,
) error {
	currentIndex := state.current
	currentCost := state.costs[currentIndex]
	currentKnown := state.known[currentIndex]
	currentPath := state.paths[currentIndex]
	currentTokenID := g.rawGraph.Tokens[currentIndex]

	if currentKnown.IsSet(uint64(currentIndex)) {
		// we should never get here!
		return errors.New("cycle detected in path history")
	}

	// Iterate through all outgoing edges from the current token.
	for _, edgeIndex := range g.rawGraph.Adjacency[currentIndex] {
		targetIndex := g.rawGraph.EdgeTargets[edgeIndex]
		targetTokenID := g.rawGraph.Tokens[targetIndex]

		// Crucial cycle prevention: do not traverse to a token that is already in the current path.
		if currentKnown.IsSet(uint64(targetIndex)) {
			continue
		}

		bestReserve := state.temp

		if !state.bestConnectionComputed.IsSet(uint64(edgeIndex)) {
			// Iterate through all pools associated with this edge.
			bestConnection := -1
			bestReserve.SetUint64(0)
			for _, poolIndex := range g.rawGraph.EdgePools[edgeIndex] {
				getReserveFunc := g.getReservesFuncs[poolIndex]
				// can be nil - @todo fix this
				if getReserveFunc == nil {
					continue
				}
				_, reserveOut, err := getReserveFunc(currentTokenID, targetTokenID)
				if err != nil {
					continue
				}

				// we need the reserveOut
				if reserveOut.Cmp(bestReserve) == 1 {
					bestReserve.Set(reserveOut)
					bestConnection = poolIndex
				}
			}

			if bestConnection != -1 {
				// we have found a best connection for this edge (the pool with the highest reserve for targetID)
				state.bestConnection[edgeIndex] = bestConnection
				state.bestConnectionComputed.Set(uint64(edgeIndex))
				state.reserveForBestConnection[edgeIndex].Set(bestReserve)
			}
		}

		if state.bestConnection[edgeIndex] != -1 {
			poolIndex := state.bestConnection[edgeIndex]
			reserve := state.reserveForBestConnection[edgeIndex]

			if state.reserves[targetIndex].Cmp(reserve) == -1 {
				amountOut, err := g.allGetAmountOutFuncs[poolIndex](currentCost, currentTokenID, targetTokenID)
				if err != nil || amountOut == nil || amountOut.Sign() <= 0 {
					continue
				}

				state.costs[targetIndex].Set(amountOut)
				poolID := g.rawGraph.Pools[poolIndex]
				newPath := make([]chains.TokenPoolPath, len(currentPath)+1)
				copy(newPath, currentPath)
				newPath[len(currentPath)] = chains.TokenPoolPath{
					TokenInID:  currentTokenID,
					TokenOutID: targetTokenID,
					PoolID:     poolID,
				}
				state.paths[targetIndex] = newPath
				state.known[targetIndex].SetFrom(currentKnown)
				state.known[targetIndex].Set(uint64(currentIndex))
				state.reserves[targetIndex].Set(reserve)
			}

		}
	}
	return nil
}

// getExchangeRatesUsingMaxAmountOutPath is the core of the algorithm. It uses the
// pre-computed swap functions for maximum performance.
// it sets connections based on maxAmountOut
func (g *Graph) getExchangeRatesUsingMaxAmountOutPath(
	state *findConversionPathState,
) error {
	currentIndex := state.current
	currentCost := state.costs[currentIndex]
	currentKnown := state.known[currentIndex]
	currentPath := state.paths[currentIndex]
	currentTokenID := g.rawGraph.Tokens[currentIndex]

	if currentKnown.IsSet(uint64(currentIndex)) {
		// we should never get here!
		return errors.New("cycle detected in path history")
	}

	// Iterate through all outgoing edges from the current token.
	for _, edgeIndex := range g.rawGraph.Adjacency[currentIndex] {
		targetIndex := g.rawGraph.EdgeTargets[edgeIndex]
		targetTokenID := g.rawGraph.Tokens[targetIndex]

		// Crucial cycle prevention: do not traverse to a token that is already in the current path.
		if currentKnown.IsSet(uint64(targetIndex)) {
			continue
		}

		bestAmountOut := state.temp
		if !state.bestConnectionComputed.IsSet(uint64(edgeIndex)) {
			// Iterate through all pools associated with this edge.
			bestConnection := -1
			bestAmountOut.SetUint64(0)
			for _, poolIndex := range g.rawGraph.EdgePools[edgeIndex] {
				getAmountOutFunc := g.allGetAmountOutFuncs[poolIndex]
				// can be nil - @todo fix this
				if getAmountOutFunc == nil {
					continue
				}
				amountOut, err := getAmountOutFunc(currentCost, currentTokenID, targetTokenID)
				if err != nil || amountOut == nil || amountOut.Sign() <= 0 {
					continue
				}

				// we need the reserveOut
				if amountOut.Cmp(bestAmountOut) == 1 {
					bestAmountOut.Set(amountOut)
					bestConnection = poolIndex
				}
			}

			if bestConnection != -1 {
				// we have found a best connection for this edge (the pool with the highest reserve for targetID)
				state.bestConnection[edgeIndex] = bestConnection
				state.bestConnectionComputed.Set(uint64(edgeIndex))
			}
		}

		if state.bestConnection[edgeIndex] != -1 {
			poolIndex := state.bestConnection[edgeIndex]
			amountOut, err := g.allGetAmountOutFuncs[poolIndex](currentCost, currentTokenID, targetTokenID)
			if err != nil || amountOut == nil || amountOut.Sign() <= 0 {
				continue
			}

			if state.costs[targetIndex].Sign() == 0 || amountOut.Cmp(state.costs[targetIndex]) == 1 {
				state.costs[targetIndex].Set(amountOut)
				poolID := g.rawGraph.Pools[poolIndex]
				newPath := make([]chains.TokenPoolPath, len(currentPath)+1)
				copy(newPath, currentPath)
				newPath[len(currentPath)] = chains.TokenPoolPath{
					TokenInID:  currentTokenID,
					TokenOutID: targetTokenID,
					PoolID:     poolID,
				}
				state.paths[targetIndex] = newPath
				state.known[targetIndex].SetFrom(currentKnown)
				state.known[targetIndex].Set(uint64(currentIndex))
			}
		}
	}
	return nil
}

// findArbitrageCyclesState encapsulates the state required for the Bellman-Ford-like
// arbitrage cycle finding algorithm.
type findArbitrageCyclesState struct {
	start         int
	current       int
	initialCost   *big.Int
	paths         [][]chains.TokenPoolPath // vertex index -> path
	costs         []*big.Int               // vertex index -> cost
	known         []bitset.BitSet          // vertex index -> vertex index
	bestCycleCost *big.Int
	temp          *big.Int
}

// FindArbitrageCycles searches the graph for a best effort at a profitable cycle
// It begins by initializing all the required fields of the findArbitrageCyclesState and
// updating our amountOut funcs with the pool overrides (if any)
func (g *Graph) FindArbitrageCycles(params chains.CycleFindingParams) ([][]chains.TokenPoolPath, []*big.Int, error) {
	runs := params.Runs
	if runs <= 0 {
		return nil, nil, errors.New("CycleFindingParams: runs must be greater than 09")
	}

	// --- Step 1: Create a temporary, patched slice of swap functions ---
	getAmountOutFuncs := make([]GetAmountOutFunc, len(g.activeGetAmountOutFuncs))
	copy(getAmountOutFuncs, g.activeGetAmountOutFuncs)

	// Patch the local function slice with V2 overrides.
	for poolID, overriddenPool := range params.UniswapV2Overrides {

		poolIndex, exists := g.poolToIndex[poolID]
		if !exists {
			continue
		}
		if getAmountOutFuncs[poolIndex] == nil {
			// pool is inactive skip!
			continue
		}
		getAmountOutFuncs[poolIndex] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
			return uniswapv2calculator.GetAmountOut(amountIn, tokenInID, tokenOutID, overriddenPool)
		}
	}

	// Patch with UniswapV3 overrides.
	for poolID, overriddenPool := range params.UniswapV3Overrides {

		poolIndex, exists := g.poolToIndex[poolID]
		if !exists {
			continue
		}
		if getAmountOutFuncs[poolIndex] == nil {
			// pool is inactive skip!
			continue
		}
		getAmountOutFuncs[poolIndex] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
			return uniswapv3calculator.GetAmountOut(amountIn, nil, tokenInID, overriddenPool)
		}
	}

	baseIndex, exists := g.tokenToIndex[params.TokenID]
	if !exists {
		return nil, nil, fmt.Errorf("token %d not found in the graph", params.TokenID)
	}

	numTokens := len(g.rawGraph.Tokens)
	state := &findArbitrageCyclesState{
		start:         baseIndex,
		initialCost:   params.AmountIn,
		paths:         make([][]chains.TokenPoolPath, numTokens),
		costs:         make([]*big.Int, numTokens),
		known:         make([]bitset.BitSet, numTokens),
		bestCycleCost: new(big.Int),
		temp:          bigIntPool.Get().(*big.Int).SetUint64(0),
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

	// Rent *big.Int objects from the pool instead of allocating new ones
	for i := range numTokens {
		state.known[i] = bitset.NewBitSet(uint64(numTokens))
		state.costs[i] = bigIntPool.Get().(*big.Int).SetUint64(0)
	}

	state.costs[baseIndex].Set(params.AmountIn)

	for range runs {
		for j := range numTokens {
			if state.costs[j].Sign() == 0 {
				continue
			}
			state.current = j
			if err := g.findArbitragePath(state, getAmountOutFuncs); err != nil {
				return nil, nil, err
			}
		}
	}

	if len(state.paths[baseIndex]) == 0 {
		return nil, nil, nil
	}

	// we set new big.Int because costs big.Ints are returned to pool
	return [][]chains.TokenPoolPath{state.paths[baseIndex]}, []*big.Int{state.bestCycleCost}, nil
}

// findArbitragePath is the core Bellman-Ford-like relaxation step for finding arbitrage.
func (g *Graph) findArbitragePath(
	state *findArbitrageCyclesState,
	getAmountOutFuncs []GetAmountOutFunc,
) error {

	currentIndex := state.current
	currentCost := state.costs[currentIndex]
	currentKnown := state.known[currentIndex]
	currentPath := state.paths[currentIndex]
	currentTokenID := g.rawGraph.Tokens[currentIndex]

	if currentKnown.IsSet(uint64(currentIndex)) {
		return nil
	}

	maxAmountOut := state.temp
	for _, edgeIndex := range g.rawGraph.Adjacency[currentIndex] {
		targetIndex := g.rawGraph.EdgeTargets[edgeIndex]
		targetTokenID := g.rawGraph.Tokens[targetIndex]
		if currentKnown.IsSet(uint64(targetIndex)) && targetIndex != state.start {
			continue
		}

		bestPoolIndex := -1
		maxAmountOut.SetUint64(0)

		for _, poolIndex := range g.rawGraph.EdgePools[edgeIndex] {
			getAmountOut := getAmountOutFuncs[poolIndex]
			// can be nil if pool is not part of active set
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

		// handle target == start separately
		if targetIndex == state.start {
			// this allows us to still collect unprofitable cycles (to be optimized up stream)
			if maxAmountOut.Cmp(state.bestCycleCost) == 1 {
				poolID := g.rawGraph.Pools[bestPoolIndex]
				newPath := make([]chains.TokenPoolPath, len(currentPath)+1)
				copy(newPath, currentPath)
				newPath[len(currentPath)] = chains.TokenPoolPath{
					TokenInID:  currentTokenID,
					TokenOutID: targetTokenID,
					PoolID:     poolID,
				}
				state.paths[targetIndex] = newPath
				state.known[targetIndex].SetFrom(currentKnown)
				state.known[targetIndex].Set(uint64(currentIndex))
				// set best cycle cost
				state.bestCycleCost.Set(maxAmountOut)
			}
		} else if maxAmountOut.Cmp(state.costs[targetIndex]) == 1 {
			poolID := g.rawGraph.Pools[bestPoolIndex]
			newPath := make([]chains.TokenPoolPath, len(currentPath)+1)
			copy(newPath, currentPath)
			newPath[len(currentPath)] = chains.TokenPoolPath{
				TokenInID:  currentTokenID,
				TokenOutID: targetTokenID,
				PoolID:     poolID,
			}
			state.paths[targetIndex] = newPath
			state.known[targetIndex].SetFrom(currentKnown)
			state.known[targetIndex].Set(uint64(currentIndex))
			state.costs[targetIndex].Set(maxAmountOut)
		}
	}

	return nil
}

// findSwapPathsState encapsulates the state required for the Bellman-Ford-like
// swap path finding algorithm.
type findSwapPathsState struct {
	start   int
	current int
	end     int
	paths   [][]chains.TokenPoolPath // vertex index -> path
	costs   []*big.Int               // vertex index -> cost
	known   []bitset.BitSet          // vertex index -> vertex index
	temp    *big.Int
}

// FindBestSwapPath searches the graph for the most profitable swap path between two tokens.
// It uses a "copy-and-patch" strategy to handle state overrides.
func (g *Graph) FindBestSwapPath(params chains.SwapFindingParams) ([]chains.TokenPoolPath, *big.Int, error) {

	// --- Step 1: Create a temporary, patched slice of swap functions ---
	getAmountOutFuncs := make([]GetAmountOutFunc, len(g.activeGetAmountOutFuncs))
	copy(getAmountOutFuncs, g.activeGetAmountOutFuncs)

	// Patch with V2 overrides.
	for poolID, overriddenPool := range params.UniswapV2Overrides {

		poolIndex, exists := g.poolToIndex[poolID]
		if !exists {
			continue
		}
		if getAmountOutFuncs[poolIndex] == nil {
			// pool is inactive skip!
			continue
		}
		getAmountOutFuncs[poolIndex] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
			return uniswapv2calculator.GetAmountOut(amountIn, tokenInID, tokenOutID, overriddenPool)
		}
	}

	// Patch with UniswapV3 overrides.
	for poolID, overriddenPool := range params.UniswapV3Overrides {

		poolIndex, exists := g.poolToIndex[poolID]
		if !exists {
			continue
		}
		if getAmountOutFuncs[poolIndex] == nil {
			// pool is inactive skip!
			continue
		}
		getAmountOutFuncs[poolIndex] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
			return uniswapv3calculator.GetAmountOut(amountIn, nil, tokenInID, overriddenPool)
		}
	}

	// --- Step 2: Initialize and run the pathfinding algorithm ---
	startIndex, exists := g.tokenToIndex[params.TokenInID]
	if !exists {
		return nil, nil, fmt.Errorf("start token %d not found in the graph", params.TokenInID)
	}

	endIndex, exists := g.tokenToIndex[params.TokenOutID]
	if !exists {
		return nil, nil, fmt.Errorf("end token %d not found in the graph", params.TokenOutID)
	}

	numTokens := len(g.rawGraph.Tokens)
	state := &findSwapPathsState{
		start: startIndex,
		end:   endIndex,
		paths: make([][]chains.TokenPoolPath, numTokens),
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

	state.costs[startIndex].Set(params.AmountIn)
	runs := params.Runs

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
	currentTokenID := g.rawGraph.Tokens[currentIndex]

	if currentKnown.IsSet(uint64(currentIndex)) {
		return errors.New("cycle detected in path history")
	}

	maxAmountOut := state.temp
	for _, edgeIndex := range g.rawGraph.Adjacency[currentIndex] {
		targetIndex := g.rawGraph.EdgeTargets[edgeIndex]

		if currentKnown.IsSet(uint64(targetIndex)) {
			continue
		}

		targetTokenID := g.rawGraph.Tokens[targetIndex]
		bestPoolIndex := -1
		maxAmountOut.SetUint64(0)
		for _, poolIndex := range g.rawGraph.EdgePools[edgeIndex] {
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
			poolID := g.rawGraph.Pools[bestPoolIndex]
			newPath := make([]chains.TokenPoolPath, len(currentPath)+1)
			copy(newPath, currentPath)
			newPath[len(currentPath)] = chains.TokenPoolPath{
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

// equalTokenPoolPaths compares two paths to see if they are identical.
func equalTokenPoolPaths(a, b []chains.TokenPoolPath) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].TokenInID != b[i].TokenInID || a[i].TokenOutID != b[i].TokenOutID || a[i].PoolID != b[i].PoolID {
			return false
		}
	}
	return true
}
