package indexer

import uniswapv2 "github.com/defistate/defistate-client-go/protocols/uniswapv2"

// IndexedUniswapV2 defines the methods for accessing indexed Uniswap V2 pool data.
type IndexedUniswapV2 interface {
	GetByID(id uint64) (uniswapv2.Pool, bool)
	All() []uniswapv2.Pool
}
