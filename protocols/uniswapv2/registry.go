package uniswapv2

import "math/big"

type Pool struct {
	ID       uint64   `json:"id"`
	Token0   uint64   `json:"token0"`
	Token1   uint64   `json:"token1"`
	Reserve0 *big.Int `json:"reserve0"`
	Reserve1 *big.Int `json:"reserve1"`
	Type     uint8    `json:"type"`
	FeeBps   uint16   `json:"feeBps"` // i.e 30 for 0.3%
}
