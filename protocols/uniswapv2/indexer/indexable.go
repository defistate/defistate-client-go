package indexer

import (
	uniswapv2 "github.com/defistate/defistate-client-go/protocols/uniswapv2"
)

// Indexer is a concrete implementation of the defistate.UniswapV2Indexer interface.
type Indexer struct{}

// New creates a new Indexer.
func New() *Indexer {
	return &Indexer{}
}

// Index creates an indexed Uniswap V2 system from a raw slice of pools.
func (i *Indexer) Index(pools []uniswapv2.Pool) IndexedUniswapV2 {
	return NewIndexableUniswapV2System(pools)
}

// IndexableUniswapV2System provides fast, indexed access to Uniswap V2 pool data.
type IndexableUniswapV2System struct {
	byID map[uint64]uniswapv2.Pool
	all  []uniswapv2.Pool
}

// NewIndexableUniswapV2System creates a new indexed Uniswap V2 system.
func NewIndexableUniswapV2System(pools []uniswapv2.Pool) *IndexableUniswapV2System {
	byID := make(map[uint64]uniswapv2.Pool, len(pools))

	for _, p := range pools {
		byID[p.ID] = p
	}

	return &IndexableUniswapV2System{
		byID: byID,
		all:  pools,
	}
}

// GetByID retrieves a pool by its unique ID.
func (ius *IndexableUniswapV2System) GetByID(id uint64) (uniswapv2.Pool, bool) {
	p, ok := ius.byID[id]
	return p, ok
}

// All returns a defensive copy of the slice of all pools.
func (ius *IndexableUniswapV2System) All() []uniswapv2.Pool {
	allCopy := make([]uniswapv2.Pool, len(ius.all))
	copy(allCopy, ius.all)
	return allCopy
}
