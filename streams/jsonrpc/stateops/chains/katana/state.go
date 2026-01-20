package katana

import (
	"encoding/json"
	"errors"

	"github.com/defistate/defistate-client-go/differ"
	"github.com/defistate/defistate-client-go/engine"
	"github.com/defistate/defistate-client-go/patcher"
	poolregistry "github.com/defistate/defistate-client-go/protocols/poolregistry"
	tokenpoolregistry "github.com/defistate/defistate-client-go/protocols/tokenpoolregistry"
	tokenregistry "github.com/defistate/defistate-client-go/protocols/tokenregistry"
	uniswapv2 "github.com/defistate/defistate-client-go/protocols/uniswapv2"
	uniswapv3 "github.com/defistate/defistate-client-go/protocols/uniswapv3"
	"github.com/prometheus/client_golang/prometheus"
)

// Logger defines a standard interface for structured, leveled logging.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// StateOps encapsulates the core business logic for processing Ethereum DSE State.
//
// It acts as a unified facade for two critical operations:
// 1. Differ: Calculating the delta between two states (Used by the Server/Engine).
// 2. Patcher: Applying a delta to a previous state to reconstruct the present (Used by a Client).
type StateOps struct {
	*differ.StateDiffer
	*patcher.StatePatcher
}

func NewStateOps(
	logger Logger,
	prometheusRegistry prometheus.Registerer,
) (*StateOps, error) {
	protocolDiffers := map[engine.ProtocolSchema]differ.ProtocolDiffer{
		tokenregistry.Schema: func(old, new any) (diff any, err error) {
			return tokenregistry.Differ(old.([]tokenregistry.Token), new.([]tokenregistry.Token)), nil
		},
		poolregistry.Schema: func(old, new any) (diff any, err error) {
			return poolregistry.Differ(old.(poolregistry.PoolRegistry), new.(poolregistry.PoolRegistry)), nil
		},
		tokenpoolregistry.Schema: func(old, new any) (diff any, err error) {
			return tokenpoolregistry.TokenPoolRegistryDiffer(old.(*tokenpoolregistry.TokenPoolRegistryView), new.(*tokenpoolregistry.TokenPoolRegistryView)), nil
		},
		uniswapv2.Schema: func(old, new any) (diff any, err error) {
			return uniswapv2.Differ(old.([]uniswapv2.Pool), new.([]uniswapv2.Pool)), nil
		},
		uniswapv3.Schema: func(old, new any) (diff any, err error) {
			return uniswapv3.Differ(old.([]uniswapv3.Pool), new.([]uniswapv3.Pool)), nil
		},
	}

	protocolPatchers := map[engine.ProtocolSchema]patcher.PatcherFunc{
		tokenregistry.Schema: func(prevState, diff any) (newState any, err error) {
			return tokenregistry.Patcher(prevState.([]tokenregistry.Token), diff.(tokenregistry.TokenSystemDiff))
		},
		poolregistry.Schema: func(prevState, diff any) (newState any, err error) {
			return poolregistry.Patcher(prevState.(poolregistry.PoolRegistry), diff.(poolregistry.PoolRegistryDiff))
		},
		tokenpoolregistry.Schema: func(prevState, diff any) (newState any, err error) {
			return tokenpoolregistry.TokenPoolRegistryPatcher(prevState.(*tokenpoolregistry.TokenPoolRegistryView), diff.(tokenpoolregistry.TokenPoolRegistryDiff))
		},
		uniswapv2.Schema: func(prevState, diff any) (newState any, err error) {
			return uniswapv2.Patcher(prevState.([]uniswapv2.Pool), diff.(uniswapv2.UniswapV2SystemDiff))
		},
		uniswapv3.Schema: func(prevState, diff any) (newState any, err error) {
			return uniswapv3.Patcher(prevState.([]uniswapv3.Pool), diff.(uniswapv3.UniswapV3SystemDiff))
		},
	}

	stateDiffer, err := differ.NewStateDiffer(&differ.StateDifferConfig{
		ProtocolDiffers: protocolDiffers,
		Logger:          logger,
		Registry:        prometheusRegistry,
	})
	if err != nil {
		return nil, err
	}

	statePatcher, err := patcher.NewStatePatcher(&patcher.StatePatcherConfig{
		Patchers: protocolPatchers,
	})
	if err != nil {
		return nil, err
	}

	return &StateOps{
		StateDiffer:  stateDiffer,
		StatePatcher: statePatcher,
	}, nil

}

func (ops *StateOps) DecodeStateJSON(
	schema engine.ProtocolSchema,
	data json.RawMessage,
) (any, error) {
	switch schema {
	case tokenregistry.Schema:
		var typedData []tokenregistry.Token
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil

	case poolregistry.Schema:
		var typedData poolregistry.PoolRegistry
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case tokenpoolregistry.Schema:
		var typedData *tokenpoolregistry.TokenPoolRegistryView
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case uniswapv2.Schema:
		var typedData []uniswapv2.Pool
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case uniswapv3.Schema:
		var typedData []uniswapv3.Pool
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	default:
		return nil, errors.New("unknown schema")
	}
}

func (ops *StateOps) DecodeStateDiffJSON(
	schema engine.ProtocolSchema,
	data json.RawMessage,
) (any, error) {
	switch schema {
	case tokenregistry.Schema:
		var typedData tokenregistry.TokenSystemDiff
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil

	case poolregistry.Schema:
		var typedData poolregistry.PoolRegistryDiff
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case tokenpoolregistry.Schema:
		var typedData tokenpoolregistry.TokenPoolRegistryDiff
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case uniswapv2.Schema:
		var typedData uniswapv2.UniswapV2SystemDiff
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case uniswapv3.Schema:
		var typedData uniswapv3.UniswapV3SystemDiff
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	default:
		return nil, errors.New("unknown schema")
	}
}
