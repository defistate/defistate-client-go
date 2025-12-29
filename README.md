# Defistate: DeFi State Stream Clients in Golang

### The public repository for Defistate's Stream clients and interactive CLI tools.

This SDK provides a high-performance entry point to the DeFI State Stream. It allows you ingest, normalize and visualize aggregated protocol data in real-time.


## Key Features
- JSON-RPC Stream Client: A headless, high-throughput client for ingesting state into your infrastructure.
- Interactive Console: A TUI (Terminal User Interface) for exploring the DeFi graph (a streamed structure that represents the connections between pools and tokens), and the aggregated protocol state provided by the stream on each block. 

## Requirements
- Go: Version 1.25.4 or higher.

## Installation
`git clone https://github.com/defistate/defi-state-client-go
cd defi-state-client-go`

## Configuration
Create a `yaml` file in the root directory, with the following fields.

`chain_id: 1                 #i.e Ethereum Mainnet
state_stream_url: "wss://your-state-stream-url"`

## Usage
There are two executables in this repository. One of them is the JSON-RPC Stream client and the other is the Console that utilizes the client and provides a CLI for visualizing and experimenting with the Stream.

### Run the Client
There are two ways to run this repository:
1. Run the Interactive Console (Recommended for Demo)

Use this to visually explore the graph, look up pools, and watch live blocks.
Bash

`go run cmd/console/main.go -config=config.yaml`

2. Run the Headless Client

Use this to stream data directly to your application logic or logs (good for background services).
Bash

`go run cmd/client/main.go -config=config.yaml`

## Architecture

The client operates on a "Protocol ID -> Schema -> Typed Data" philosophy.

    - Pool Registry: Maps every Pool Address to a unique uint64 ID.

    - Token System: Maps Tokens to IDs and metadata.

    - Token-Pool Graph: A pre-computed, traversable graph connecting Tokens to Pools.

    - Live Protocols: Normalized state (Ticks, Reserves) synchronized to the latest block.

For a deeper dive into the architecture, run the console and press h (Help).