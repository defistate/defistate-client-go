package indexer

import (
	tokenregistry "github.com/defistate/defistate-client-go/protocols/tokenregistry"
	"github.com/ethereum/go-ethereum/common"
)

// Indexer is a concrete implementation of the defistate.TokenIndexer interface.
type Indexer struct{}

// New creates a new Indexer.
func New() *Indexer {
	return &Indexer{}
}

// Index creates an indexed tokenregistry system from a raw slice of tokens.
func (i *Indexer) Index(tokens []tokenregistry.Token) IndexedTokenSystem {
	return NewIndexableTokenSystem(tokens)
}

// IndexableTokenSystem provides fast, indexed access to tokenregistry data.
type IndexableTokenSystem struct {
	byID      map[uint64]tokenregistry.Token
	byAddress map[common.Address]tokenregistry.Token
	all       []tokenregistry.Token
}

// NewIndexableTokenSystem creates a new indexed tokenregistry system from a raw slice.
func NewIndexableTokenSystem(tokens []tokenregistry.Token) *IndexableTokenSystem {
	byID := make(map[uint64]tokenregistry.Token, len(tokens))
	byAddress := make(map[common.Address]tokenregistry.Token, len(tokens))

	for _, t := range tokens {
		byID[t.ID] = t
		byAddress[t.Address] = t
	}

	return &IndexableTokenSystem{
		byID:      byID,
		byAddress: byAddress,
		all:       tokens,
	}
}

// GetByID retrieves a tokenregistry by its unique ID.
func (its *IndexableTokenSystem) GetByID(id uint64) (tokenregistry.Token, bool) {
	t, ok := its.byID[id]
	return t, ok
}

// GetByAddress retrieves a tokenregistry by its contract address.
func (its *IndexableTokenSystem) GetByAddress(address common.Address) (tokenregistry.Token, bool) {
	t, ok := its.byAddress[address]
	return t, ok
}

// All returns a defensive copy of the slice of all tokens in the system.
func (its *IndexableTokenSystem) All() []tokenregistry.Token {
	allCopy := make([]tokenregistry.Token, len(its.all))
	copy(allCopy, its.all)
	return allCopy
}
