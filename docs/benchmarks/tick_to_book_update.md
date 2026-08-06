# Tick-to-book-update latency

Measures `KrakenBookClient::handleMessage`'s parse + book-apply + checksum
cost in isolation.

## Release build

| Speed | count | p50 | p90 | p99 | max |
|---|---|---|---|---|---|
| burst | 1113 | 4.1μs | 4.1μs | 8.2μs | 126.1μs |
| real | 1113 | 32.8μs | 32.8μs | 65.5μs | 256.4μs |

## Debug build

| Speed | count | p50 | p90 | p99 | max |
|---|---|---|---|---|---|
| burst | 1113 | 65.5μs | 65.5μs | 131μs | 736μs |
| real | 1113 | 131μs | 262μs | 262μs | 1.47ms |
