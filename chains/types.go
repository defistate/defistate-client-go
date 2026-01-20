package chains

import (
	"math/big"

	"github.com/defistate/defistate-client-go/engine"
	poolregistry "github.com/defistate/defistate-client-go/protocols/poolregistry"
	poolregistryindexer "github.com/defistate/defistate-client-go/protocols/poolregistry/indexer"

	tokenpoolregistry "github.com/defistate/defistate-client-go/protocols/tokenpoolregistry"
	tokenregistry "github.com/defistate/defistate-client-go/protocols/tokenregistry"
	tokenregistryindexer "github.com/defistate/defistate-client-go/protocols/tokenregistry/indexer"
	uniswapv2 "github.com/defistate/defistate-client-go/protocols/uniswapv2"
	uniswapv2indexer "github.com/defistate/defistate-client-go/protocols/uniswapv2/indexer"
	uniswapv3 "github.com/defistate/defistate-client-go/protocols/uniswapv3"
	uniswapv3indexer "github.com/defistate/defistate-client-go/protocols/uniswapv3/indexer"
)

// Logger defines a standard interface for structured, leveled logging.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Client defines the interface that DefiState depends on.
type Client interface {
	State() <-chan *engine.State
	Err() <-chan error
}

// TokenIndexer defines the interface for any component that can index tokens.
type TokenIndexer interface {
	Index(tokens []tokenregistry.Token) tokenregistryindexer.IndexedTokenSystem
}

// PoolRegistryIndexer defines the interface for any component that can index poolregistry registries.
type PoolRegistryIndexer interface {
	Index(poolregistry.PoolRegistry) poolregistryindexer.IndexedPoolRegistry
}

// UniswapV2Indexer defines the interface for any component that can index Uniswap V2 pools.
type UniswapV2Indexer interface {
	Index(pools []uniswapv2.Pool) uniswapv2indexer.IndexedUniswapV2
}

// UniswapV3Indexer defines the interface for any component that can index Uniswap V3 pools.
type UniswapV3Indexer interface {
	Index(pools []uniswapv3.Pool) uniswapv3indexer.IndexedUniswapV3
}

type TokenPoolPath struct {
	TokenInID  uint64
	TokenOutID uint64
	PoolID     uint64
}

// CycleFindingParams encapsulates all inputs for an arbitrage search.
type CycleFindingParams struct {
	AmountIn *big.Int
	TokenID  uint64

	// Overrides allow for "what-if" analysis by providing a modified
	// state for specific pools, which will be used instead of the
	// state from the main view. The key is the poolregistry ID.
	UniswapV2Overrides map[uint64]uniswapv2.Pool
	UniswapV3Overrides map[uint64]uniswapv3.Pool
	Runs               int // Number of runs to perform in the search.
}

// CycleFindingParamsFromStartPool encapsulates all inputs for an arbitrage search from a specific poolregistry
type CycleFindingParamsFromStartPool struct {
	AmountIn   *big.Int
	PoolID     uint64
	TokenInID  uint64
	TokenOutID uint64

	// Overrides allow for "what-if" analysis by providing a modified
	// state for specific pools, which will be used instead of the
	// state from the main view. The key is the poolregistry ID.
	UniswapV2Overrides map[uint64]uniswapv2.Pool
	UniswapV3Overrides map[uint64]uniswapv3.Pool
	Runs               int // Number of runs to perform in the search.
}

// SwapFindingParams encapsulates all inputs for finding the best swap path.
type SwapFindingParams struct {
	AmountIn   *big.Int
	TokenInID  uint64
	TokenOutID uint64
	Runs       int // Number of runs to perform in the search.

	// Overrides allow for "what-if" analysis.
	UniswapV2Overrides map[uint64]uniswapv2.Pool
	UniswapV3Overrides map[uint64]uniswapv3.Pool
}

// TokenPoolGraph provides the complete interface for querying the analytical graph.
type TokenPoolGraph interface {
	GetPoolsForToken(tokenID uint64) (pools []uint64, err error)
	GetTokensForPool(poolID uint64) (tokens []uint64, err error)
	GetExchangeRates(
		baseAmountIn *big.Int,
		baseTokenID uint64,
		runs int,
		allowedSourceTokens map[uint64]struct{},
	) (map[uint64]*big.Int, error)
	FindArbitrageCycles(params CycleFindingParams) ([][]TokenPoolPath, []*big.Int, error)
	FindBestSwapPath(params SwapFindingParams) ([]TokenPoolPath, *big.Int, error)
	Raw() *tokenpoolregistry.TokenPoolRegistryView
}

type TokenPoolGrapher interface {
	Graph(
		tokenPool *tokenpoolregistry.TokenPoolRegistryView,
		indexedTokenRegistry tokenregistryindexer.IndexedTokenSystem,
		indexedPoolRegistry poolregistryindexer.IndexedPoolRegistry,
		indexedUniswapV2 uniswapv2indexer.IndexedUniswapV2,
		indexedUniswapV3 uniswapv3indexer.IndexedUniswapV3,
		protocolResolver *ProtocolResolver,
	) (TokenPoolGraph, error)
}

// ProtocolResolver handles the resolution of high-level protocol schemas
// from low-level poolregistry identifiers. It centralizes the multi-step lookup logic.
type ProtocolResolver struct {
	protocolIDToSchema  map[engine.ProtocolID]engine.ProtocolSchema
	indexedPoolRegistry poolregistryindexer.IndexedPoolRegistry
}

// NewProtocolResolver creates a new resolver instance.
func NewProtocolResolver(
	protocolIDToSchema map[engine.ProtocolID]engine.ProtocolSchema,
	registry poolregistryindexer.IndexedPoolRegistry,
) *ProtocolResolver {
	return &ProtocolResolver{
		protocolIDToSchema:  protocolIDToSchema,
		indexedPoolRegistry: registry,
	}
}

// ResolveSchemaFromPoolID performs the full lookup chain to find the
// data schema for a specific poolregistry ID.
//
// Lookup Chain:
// 1. PoolID -> Pool (via Registry)
// 2. Pool.Protocol (uint16) -> ProtocolID (string) (via Registry Mapping)
// 3. ProtocolID -> ProtocolSchema (via Engine Config)
func (pr *ProtocolResolver) ResolveSchemaFromPoolID(poolID uint64) (engine.ProtocolSchema, bool) {
	// 1. Get the poolregistry from the registry
	poolregistry, ok := pr.indexedPoolRegistry.GetByID(poolID)
	if !ok {
		return "", false
	}

	// 2. Get the protocol map from the registry
	// Optimization Note: This assumes GetProtocols() is reasonably fast (e.g. cached map).
	protocols := pr.indexedPoolRegistry.GetProtocols()

	// 3. Resolve the internal uint16 ID to the engine's string ID
	protocolID, ok := protocols[poolregistry.Protocol]
	if !ok {
		return "", false
	}

	// 4. Resolve the string ID to the schema
	schema, ok := pr.protocolIDToSchema[protocolID]
	return schema, ok
}

// ResolveSchema directly maps a known ProtocolID string to its schema.
func (pr *ProtocolResolver) ResolveSchema(protocolID engine.ProtocolID) (engine.ProtocolSchema, bool) {
	schema, exists := pr.protocolIDToSchema[protocolID]
	return schema, exists
}
