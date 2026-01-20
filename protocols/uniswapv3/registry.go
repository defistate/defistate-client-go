package uniswapv3

import (
	"math/big"
)

// @note do not change the PoolViewMinimal struct name until it is confirmed from the uniswap v3 indexer
// PoolViewMinimal provides a view of a single Uniswap V3 pool's data.
type PoolViewMinimal struct {
	ID           uint64   `json:"id"`
	Token0       uint64   `json:"token0"`
	Token1       uint64   `json:"token1"`
	Fee          uint64   `json:"fee"`
	TickSpacing  uint64   `json:"tickSpacing"`
	Tick         int64    `json:"tick"`
	Liquidity    *big.Int `json:"liquidity"`
	SqrtPriceX96 *big.Int `json:"sqrtPriceX96"`
}

// TickInfo represents the information about a tick in a Uniswap V3 pool.
// i know big.Int is not the most cache-friendly type, but it is accurate and required for this implementation
// it will be replaced in the future.
type TickInfo struct {
	Index          int64    `json:"index"`
	LiquidityGross *big.Int `json:"liquidityGross"`
	LiquidityNet   *big.Int `json:"liquidityNet"`
	// all we care about for now are the liquidity fields
	//FeeGrowthOutside0x128           *big.Int
	//FeeGrowthOutside1x128           *big.Int
	//TickCumulativeOutside           *big.Int
	//SecondsPerLiquidityOutside0x128 *big.Int
	//SecondsOutside                  *big.Int
	//Initialized                     bool -presence of this object implicitly means tick is initialized
}

// Pool is the fully enriched view of a pool, combining the minimal
// core data with the detailed tick liquidity information.
type Pool struct {
	PoolViewMinimal `json:",inline"`
	Ticks           []TickInfo `json:"ticks"`
}
