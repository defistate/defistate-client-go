package indexer

import uniswapv3 "github.com/defistate/defistate-client-go/protocols/uniswapv3"

// IndexedUniswapV3 provides a unified, read-only view of all indexed Uniswap V3
// and V3-like pools. As the output of the UniswapV3Indexer, it contains merged
// data from the primary protocol and its forks, offering a consolidated state
// for querying.
// Always check Pool.Type to confirm the actual pool type
type IndexedUniswapV3 interface {
	GetByID(id uint64) (uniswapv3.Pool, bool)
	All() []uniswapv3.Pool
}
