package engine

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type ProtocolName string
type ProtocolID string

// ProtocolSchem defines the decode contract for a protocol's data
type ProtocolSchema string

type ProtocolMeta struct {
	Name ProtocolName `json:"name"`           // human label
	Tags []string     `json:"tags,omitempty"` // "dex", "fork", etc.
}

type ProtocolState struct {
	Meta ProtocolMeta `json:"meta"`

	// what is the current block of the protocol's data?
	SyncedBlockNumber *uint64 `json:"syncedBlockNumber,omitempty"`

	// Schema is the decode contract for Data.
	// Example:
	// "defistate/uniswap-v2-system/PoolView@v1"
	Schema ProtocolSchema `json:"schema"`

	// Data is the protocol view, shaped by Schema.
	Data any `json:"data,omitempty"`

	// Error is populated if this protocol is out-of-sync or failed for this block.
	Error string `json:"error,omitempty"`
}

// BlockSummary contains only the essential block information for clients.
type BlockSummary struct {
	Number      *big.Int    `json:"number"`
	Hash        common.Hash `json:"hash"`
	Timestamp   uint64      `json:"timestamp"`
	ReceivedAt  int64       `json:"receivedAt"` // The Unix nanosecond timestamp when the engine started processing the block.
	GasUsed     uint64      `json:"gasUsed"`
	GasLimit    uint64      `json:"gasLimit"`
	StateRoot   common.Hash `json:"stateRoot"`
	TxHash      common.Hash `json:"txHash"`
	ReceiptHash common.Hash `json:"receiptHash"`
}

// State is the main data structure broadcast to subscribers.
type State struct {
	ChainID   uint64                       `json:"chainId"`
	Timestamp uint64                       `json:"timestamp"`
	Block     BlockSummary                 `json:"block"`
	Protocols map[ProtocolID]ProtocolState `json:"protocols"`
}

func (state *State) HasErrors() bool {
	// Check protocol-level errors
	for _, pr := range state.Protocols {
		if pr.Error != "" {
			return true
		}
	}
	return false
}
