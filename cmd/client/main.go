package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/defistate/defistate-client-go/cmd/client/config"
	"github.com/defistate/defistate-client-go/differ"
	"github.com/defistate/defistate-client-go/engine"
	"github.com/defistate/defistate-client-go/streams/jsonrpc/client"
	"github.com/defistate/defistate-client-go/streams/jsonrpc/stateops/chains"
	ethstateops "github.com/defistate/defistate-client-go/streams/jsonrpc/stateops/chains/ethereum"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	DefaultClientStateBufferSize = 100
)

type ChainStateOps interface {
	Diff(old *engine.State, new *engine.State) (*differ.StateDiff, error)
	Patch(oldState *engine.State, diff *differ.StateDiff) (*engine.State, error)
	DecodeStateJSON(schema engine.ProtocolSchema, data json.RawMessage) (any, error)
	DecodeStateDiffJSON(schema engine.ProtocolSchema, data json.RawMessage) (any, error)
}

func main() {
	// create the log handler
	rootLogHandler := slog.NewJSONHandler(os.Stdout, nil)
	close := func() {
		os.Exit(1)
	}

	rootLogger := slog.New(rootLogHandler)
	prometheusRegistry := prometheus.DefaultRegisterer
	cfg, err := loadConfig()
	if err != nil {
		rootLogger.Error("Failed to load configuration", "error", err)
		close()
	}

	// Create a context that cancels when the OS sends an interrupt (Ctrl+C) or termination signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var (
		chainStateOps ChainStateOps
	)

	switch cfg.ChainID.Uint64() {
	case chains.Mainnet:
		chainStateOps, err = ethstateops.NewStateOps(rootLogger, prometheusRegistry)
		if err != nil {
			rootLogger.Error("Failed to initialize Chain State Ops", "chain_id", cfg.ChainID, "error", err)
			close()
		}
	default:
		// we don't know this chain, log error and close.
		rootLogger.Error(fmt.Errorf("Chain State Ops not found for chain with ID %d", cfg.ChainID.Uint64()).Error())
		close()
	}

	client, err := client.NewClient(
		ctx,
		client.Config{
			URL:              cfg.StateStreamURL,
			Logger:           rootLogger.With("component", "jsonrpc-client"),
			BufferSize:       DefaultClientStateBufferSize,
			StatePatcher:     chainStateOps.Patch,
			StateDecoder:     chainStateOps.DecodeStateJSON,
			StateDiffDecoder: chainStateOps.DecodeStateDiffJSON,
		},
	)

	if err != nil {
		rootLogger.Error("Failed to initialize Client", "chain_id", cfg.ChainID, "error", err)
		close()
	}

	for {
		select {
		case <-client.State():
		// consume state
		case err := <-client.Err():
			rootLogger.Error("Fatal client error", "error", err)
			return //
		case <-ctx.Done():
			return
		}
	}

}

func loadConfig() (*config.ClientConfig, error) {
	configPath := flag.String("config", "config.yaml", "Path to the configuration file.")
	flag.Parse()
	log.Printf("Loading configuration from: %s", *configPath)
	return config.LoadConfig(*configPath)
}
