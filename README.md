# Real-Time Paper Trading Platform

Paper trading on live Kraken market data. A C++ engine matches simulated orders
against a real order book; a Go backend streams candles, bridges everything to
the browser over WebSocket, and persists fills; a React frontend renders the
depth ladder, chart, and order terminal.

![Demo](assets/demo.gif)

## Architecture

```
 Kraken WS/REST                                  Kraken WS/REST
 (book channel)                                 (ohlc channel + history)
       │                                               │
       ▼                                               ▼
┌─────────────┐                                 ┌─────────────┐    WebSocket    ┌──────────┐
│  C++ engine │──── shared memory ─────────────▶│  Go backend │◀───────────────▶│ frontend │
│ (order book,│       (snapshots + deltas)      │ (book+candle│                 │ (React/  │
│  matching)  │◀──────── gRPC control plane ────│  fan-out,   │                 │  Vite)   │
└─────────────┘   SubscribeBook / PlaceOrder /  │  orders/DB) │                 └──────────┘
                           WatchFills           └─────────────┘
```

Three independent processes, so the engine can be restarted or debugged without
touching the web-facing backend. Details: [engine](engine/README.md),
[backend](backend/README.md).

## Quickstart (Docker)

```
docker compose up -d
```

Starts everything: engine (`:50051`), backend (`localhost:8000`), frontend
(`localhost:5173`). First build compiles the engine's C++ deps from source
(~20-60 min)

## Local development

Prereqs: CMake 3.20+, Ninja, a C++20 compiler, and
[vcpkg](https://github.com/microsoft/vcpkg) for the engine; Go 1.26+ for the
backend; Node 22+ for the frontend.

### Engine

```bash
cmake --preset release -S engine -DCMAKE_TOOLCHAIN_FILE=<vcpkg>/scripts/buildsystems/vcpkg.cmake
cmake --build engine/out/build/release --target engine
engine/out/build/release/engine.exe
```

### Backend

```bash
bash scripts/gen_proto.sh   # first run only -- needs protoc on PATH, genproto/ is gitignored
cd backend && go run ./cmd/server
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

### All three at once

```bash
bash scripts/run_dev.sh
```

Expects a `debug-local` engine preset, which isn't one of the tracked presets
in `engine/CMakePresets.json` -- define your own or edit the script.

## Testing

```bash
# engine -- engine_tests only builds/runs on Windows (the ring-buffer stress
# test uses a Windows-only spawn API); Linux only builds the `engine` target
cmake --build engine/out/build/<preset> --target engine_tests kraken_replayer
ctest --test-dir engine/out/build/<preset> --output-on-failure

# backend
cd backend && go test ./...
```

The backend's cross-process test spawns a real engine binary and checks the C++
writer and Go reader agree on the shared-memory layout end to end. It's opt-in
(skipped unless `PAPER_TRADER_TEST_ENGINE_BIN` is set) and needs an absolute path,
since `go test` runs from the package directory:

```bash
cmake --build engine/out/build/<preset> --target test_engine_harness

# bash
PAPER_TRADER_TEST_ENGINE_BIN="$(pwd)/../engine/out/build/<preset>/tools/replay/test_engine_harness" \
  go test -run CrossProcess -v ./internal/integration/...

# PowerShell
$env:PAPER_TRADER_TEST_ENGINE_BIN = (Resolve-Path "..\engine\out\build\<preset>\tools\replay\test_engine_harness.exe").Path
go test -run CrossProcess -v ./internal/integration/...
```

## Benchmarks & replay tooling

Measured p50/p90/p99 latency numbers for the engine's hot paths live in
[`docs/benchmarks/`](docs/benchmarks/README.md). The same tooling
(`kraken_recorder`, `kraken_replayer`) records and replays Kraken sessions for
load testing and golden-checksum regression tests.

## Project structure

```
engine/               C++ matching engine
  src/, include/        order book, matching engine, Kraken book-WS client, IPC, gRPC server
  proto/                 paper_trader.proto -- shared gRPC contract with the backend
  tests/                 Catch2 test suite (engine_tests)
  tools/replay/          kraken_recorder, kraken_replayer, test_engine_harness

backend/              Go backend
  cmd/server/            entrypoint
  internal/book/         shared-memory reader, WS fan-out, backpressure/resync
  internal/candle/       independent Kraken OHLC WS client + REST history
  internal/trading/      order placement, fill streaming, engine-restart reconciliation
  internal/persistence/  SQLite order/fill storage, FIFO position/P&L accounting
  internal/integration/  cross-package/cross-process tests

frontend/             React + Vite + TypeScript UI
  src/components/        depth ladder, candle chart, order entry, fills, P&L
  src/hooks/             one WebSocket hook per channel (book/candle/control)
  src/state/             BookStore (client-side snapshot+delta mirror)

docs/benchmarks/      measured p50/p90/p99 latency numbers
scripts/              run_dev.sh, record_kraken_feed.sh
```

## API

Prices/sizes are integer ticks (fixed point) everywhere except `place_order`,
which takes decimal strings.

- HTTP: `GET /health`, `GET /api/symbols`, `GET /api/candles/history`
- WebSocket: `/ws/book`, `/ws/candles`, `/ws/control`
- gRPC (engine ↔ backend, internal): `SubscribeBook`, `PlaceOrder`,
  `CancelOrder`, `WatchFills`, `GetEngineInfo`
