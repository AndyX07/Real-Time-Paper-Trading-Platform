# Engine

One gRPC server, one thread per subscribed symbol running a dedicated
`KrakenBookClient` + `MatchingEngine` pair, isolated per symbol.

- **Correctness.** Book messages apply to a sorted-vector book (100
  levels/side), verified via CRC32 against Kraken's own checksum (top 10
  levels); a mismatch triggers unsubscribe/resubscribe for a fresh
  snapshot. Also republishes a full snapshot every 500 deltas or 5s.
- **Resilience.** TLS 1.3 WS to Kraken; reconnects with backoff
  (500ms→32s), 10s connect timeout, 60s read-idle watchdog.
- **Matching.** Market orders (and the crossing part of limit orders) fill
  synchronously, walking the book depth-first. Resting limit orders track a
  `queueAheadSize` and only fill once book-size reductions at that price
  overflow it — price-time priority without visible order IDs.
- **IPC.** Per-symbol shared-memory slot: a seqlock snapshot + a
  4096-entry SPSC ring buffer for deltas, which drops-and-counts (never
  overwrites) on overflow.
- **Control plane.** gRPC: `SubscribeBook`/`UnsubscribeBook`,
  `PlaceOrder`/`CancelOrder` (30s timeout), `WatchFills` (streamed,
  pushed immediately), `GetEngineInfo` (a random per-startup instance ID
  the backend uses to detect restarts).
- **Observability.** Latency histograms on the hot paths, logged every
  30s. `tools/replay/` records/replays sessions for benchmarks and
  golden-checksum regression tests.

See the [root README](../README.md) for build/run/test instructions and
the [API reference](../README.md#api-reference) for the gRPC contract.
