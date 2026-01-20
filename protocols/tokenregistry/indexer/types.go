package indexer

import (
	tokenregistry "github.com/defistate/defistate-client-go/protocols/tokenregistry"
	"github.com/ethereum/go-ethereum/common"
)

// IndexedTokenSystem defines the methods for accessing indexed tokenregistry data.
type IndexedTokenSystem interface {
	GetByID(id uint64) (tokenregistry.Token, bool)
	GetByAddress(address common.Address) (tokenregistry.Token, bool)
	All() []tokenregistry.Token
}
