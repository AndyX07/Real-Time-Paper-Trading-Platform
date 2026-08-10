# Backend

Three independent pipelines — `internal/book`, `internal/candle`,
`internal/trading` — sharing nothing but a logger and shutdown context.

- **Book.** Polls the engine's shared-memory segment directly (`mmap`),
  tracking a per-symbol sequence number; a gap triggers a resync (re-read
  snapshot, rebuild mirror). A full client outbox forces a fresh snapshot
  and disconnects the client after 5 overflows/10s. Resyncs/overflows are
  counted, exposed at `/debug/book_counters`.
- **Candle.** A fully separate WS connection to Kraken's `ohlc` channel
  (backoff 1s→64s, subscriptions replayed on reconnect), plus a one-time
  REST backfill (`/api/candles/history`, 720 bars). Overflow policy
  differs from book: drops the oldest queued message, not the newest.
- **Trading.** Routes `PlaceOrder`/`CancelOrder` to the engine over gRPC;
  persists every order/fill/position to SQLite and broadcasts to all
  clients (one shared account). Positions/P&L are FIFO-matched from the
  full fill history on every read. A separate `WatchFills` loop (backoff
  500ms→32s) reconciles engine restarts by canceling orders the backend
  still thinks are open.

See the [root README](../README.md) for build/run/test instructions and
the [API reference](../README.md#api-reference) for HTTP/WebSocket
message shapes.
