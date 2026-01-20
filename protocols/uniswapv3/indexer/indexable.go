package indexer

import (
	uniswapv3 "github.com/defistate/defistate-client-go/protocols/uniswapv3"
)

// Indexer is a concrete implementation of the defistate.UniswapV3Indexer interface.
type Indexer struct{}

// New creates a new Indexer.
func New() *Indexer {
	return &Indexer{}
}

// Index creates an indexed Uniswap V3 system from a raw slice of pools.
func (i *Indexer) Index(pools []uniswapv3.Pool) IndexedUniswapV3 {
	return NewIndexableUniswapV3System(pools)
}

// IndexableUniswapV3System provides fast, indexed access to Uniswap V3 pool data.
type IndexableUniswapV3System struct {
	byID map[uint64]uniswapv3.Pool
	all  []uniswapv3.Pool
}

// NewIndexableUniswapV3System creates a new indexed Uniswap V3 system.
func NewIndexableUniswapV3System(pools []uniswapv3.Pool) *IndexableUniswapV3System {
	byID := make(map[uint64]uniswapv3.Pool, len(pools))

	for _, p := range pools {
		byID[p.ID] = p
	}

	return &IndexableUniswapV3System{
		byID: byID,
		all:  pools,
	}
}

// GetByID retrieves a pool by its unique ID.
func (ius *IndexableUniswapV3System) GetByID(id uint64) (uniswapv3.Pool, bool) {
	p, ok := ius.byID[id]
	return p, ok
}

// All returns a defensive copy of the slice of all pools.
func (ius *IndexableUniswapV3System) All() []uniswapv3.Pool {
	allCopy := make([]uniswapv3.Pool, len(ius.all))
	copy(allCopy, ius.all)
	return allCopy
}
