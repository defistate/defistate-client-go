package grapher

import (
	"github.com/defistate/defistate-client-go/chains"
	poolregistryindexer "github.com/defistate/defistate-client-go/protocols/poolregistry/indexer"
	tokenpoolregistry "github.com/defistate/defistate-client-go/protocols/tokenpoolregistry"
	tokenregistryindexer "github.com/defistate/defistate-client-go/protocols/tokenregistry/indexer"
	"github.com/defistate/defistate-client-go/protocols/uniswapv2"
	uniswapv2indexer "github.com/defistate/defistate-client-go/protocols/uniswapv2/indexer"
	"github.com/defistate/defistate-client-go/protocols/uniswapv3"
	uniswapv3indexer "github.com/defistate/defistate-client-go/protocols/uniswapv3/indexer"
)

var _ chains.TokenPoolGrapher = &Grapher{}

type Grapher struct {
}

func NewGrapher() (*Grapher, error) {
	grapher := &Grapher{}
	return grapher, nil
}

func (g *Grapher) Graph(
	rawGraph *tokenpoolregistry.TokenPoolRegistryView,
	tokenregistry tokenregistryindexer.IndexedTokenSystem,
	indexedPoolRegistry poolregistryindexer.IndexedPoolRegistry,
	indexedUniswapV2 uniswapv2indexer.IndexedUniswapV2,
	indexedUniswapV3 uniswapv3indexer.IndexedUniswapV3,
	protocolResolver *chains.ProtocolResolver,
) (chains.TokenPoolGraph, error) {
	// we will set pools without tokens with fee as active

	activePools := make(map[uint64]struct{})
	for _, pool := range indexedPoolRegistry.All() {
		schema, ok := protocolResolver.ResolveSchemaFromPoolID(pool.ID)
		if !ok {
			continue
		}

		// simple check for valid pools (must not contain fee on transfer tokens)
		// other checks can be implemented
		isValidPool := false
		switch schema {
		case uniswapv2.Schema:
			uniswapV2Pool, ok := indexedUniswapV2.GetByID(pool.ID)
			if !ok {
				continue
			}

			token0, ok := tokenregistry.GetByID(uniswapV2Pool.Token0)
			if !ok {
				continue
			}
			token1, ok := tokenregistry.GetByID(uniswapV2Pool.Token1)
			if !ok {
				continue
			}

			// filter out tokens with fee
			if token0.FeeOnTransferPercent > 0 || token1.FeeOnTransferPercent > 0 {
				continue
			}

			isValidPool = true

		case uniswapv3.Schema:
			uniswapV3Pool, ok := indexedUniswapV3.GetByID(pool.ID)
			if !ok {
				continue
			}

			token0, ok := tokenregistry.GetByID(uniswapV3Pool.Token0)
			if !ok {
				continue
			}
			token1, ok := tokenregistry.GetByID(uniswapV3Pool.Token1)
			if !ok {
				continue
			}

			// filter out tokens with fee
			if token0.FeeOnTransferPercent > 0 || token1.FeeOnTransferPercent > 0 {
				continue
			}
			isValidPool = true

		}

		if isValidPool {
			activePools[pool.ID] = struct{}{}
		}
	}

	return NewGraph(
		rawGraph,
		indexedPoolRegistry,
		indexedUniswapV2,
		indexedUniswapV3,
		activePools,
		protocolResolver,
	)
}
