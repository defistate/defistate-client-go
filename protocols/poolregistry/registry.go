package poolregistry

import "github.com/defistate/defistate-client-go/engine"

// Pool represents the data for a single pool.
type Pool struct {
	ID       uint64  `json:"id"`
	Key      PoolKey `json:"key"`      // Renamed from Identifier
	Protocol uint16  `json:"protocol"` // Internal uint16 representation
}

// PoolRegistry represents the complete state of the registry.
type PoolRegistry struct {
	Pools     []Pool                       `json:"pools"`
	Protocols map[uint16]engine.ProtocolID `json:"protocols"`
}
