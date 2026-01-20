package indexer

import (
	"github.com/defistate/defistate-client-go/engine"
	poolregistry "github.com/defistate/defistate-client-go/protocols/poolregistry"
	"github.com/ethereum/go-ethereum/common"
)

// IndexedPoolRegistry defines the methods for accessing indexed pool registry data.
type IndexedPoolRegistry interface {
	GetByID(id uint64) (poolregistry.Pool, bool)
	GetByAddress(address common.Address) (poolregistry.Pool, bool)
	GetByPoolKey(key poolregistry.PoolKey) (poolregistry.Pool, bool)
	All() []poolregistry.Pool
	GetProtocols() map[uint16]engine.ProtocolID
}
