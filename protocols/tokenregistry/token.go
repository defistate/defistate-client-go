package tokenregistry

import "github.com/ethereum/go-ethereum/common"

// Token is a safe, structured representation of a token's data for external use.
type Token struct {
	ID                   uint64         `json:"id"`
	Address              common.Address `json:"address"`
	Name                 string         `json:"name"`
	Symbol               string         `json:"symbol"`
	Decimals             uint8          `json:"decimals"`
	FeeOnTransferPercent float64        `json:"feeOnTransferPercent"`
	GasForTransfer       uint64         `json:"gasForTransfer"`
}
