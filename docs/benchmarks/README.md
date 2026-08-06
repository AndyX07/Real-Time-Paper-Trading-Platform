# Benchmarks

Real numbers from running the replay harness (`engine/tools/replay`) against a
recorded live Kraken BTC/USD session.

## Methodology

- **Hardware:** AMD Ryzen 7 260, Windows 11 Home.
- **Build:** MSVC (Visual Studio 2022), a Release-type CMake preset
  (optimized) — the numbers reported here. A Debug-type build of the exact same session is also recorded, for comparison.
- **Input:** a 60-second recorded BTC/USD session (1113 real Kraken WS
  frames), captured via `scripts/record_kraken_feed.sh` /
  `kraken_recorder`.
- **Tool:** `kraken_replayer bench --in <session> --speed <real|burst>
  --measure-pop`, which feeds the recording into the real
  `KrakenBookClient::handleMessage` path and dumps `HistogramRegistry`'s
  p50/p90/p99/max on completion. Reproduce with:
  ```
  engine/out/build/<preset-name>/tools/replay/kraken_replayer.exe bench --in <session.ptrec> --speed burst --measure-pop
  ```
- Two speeds were run: **burst** (frames fed back-to-back, no pacing — the
  worst-case/max-throughput scenario) and **real** (paced to the original
  inter-frame gaps — closer to production conditions).

## Results

See [tick_to_book_update.md](tick_to_book_update.md) and [ring_buffer_push_pop.md](ring_buffer_push_pop.md)
