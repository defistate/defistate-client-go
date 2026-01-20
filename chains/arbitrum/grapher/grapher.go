package grapher

import (
	"github.com/defistate/defistate-client-go/chains"
	poolregistryindexer "github.com/defistate/defistate-client-go/protocols/poolregistry/indexer"
	tokenpoolregistry "github.com/defistate/defistate-client-go/protocols/tokenpoolregistry"
	uniswapv2indexer "github.com/defistate/defistate-client-go/protocols/uniswapv2/indexer"
	uniswapv3indexer "github.com/defistate/defistate-client-go/protocols/uniswapv3/indexer"
)

var _ chains.TokenPoolGrapher = &Grapher{}

// Grapher is the central, stateful factory for creating Graph objects.
type Grapher struct {
}

// NewGrapher now accepts the Config struct for cleaner dependency injection.
func NewGrapher() (*Grapher, error) {
	grapher := &Grapher{}
	return grapher, nil
}

// Graph method now uses a Read Lock for better performance.
func (g *Grapher) Graph(
	rawGraph *tokenpoolregistry.TokenPoolRegistryView,
	poolRegistry poolregistryindexer.IndexedPoolRegistry,
	indexedUniswapV2 uniswapv2indexer.IndexedUniswapV2,
	indexedUniswapV3 uniswapv3indexer.IndexedUniswapV3,
	protocolResolver *chains.ProtocolResolver,
) (chains.TokenPoolGraph, error) {
	// we will set all pools as active

	activePools := make(map[uint64]struct{})
	for _, pool := range poolRegistry.All() {
		activePools[pool.ID] = struct{}{}
	}

	return NewGraph(
		rawGraph,
		poolRegistry,
		indexedUniswapV2,
		indexedUniswapV3,
		activePools,
		protocolResolver,
	)
}
