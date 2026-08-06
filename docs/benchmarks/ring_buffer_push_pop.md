# Ring buffer push/pop latency

Isolates the shared-memory IPC.

## Release build

| Speed | Op | count | p50 | p90 | p99 | max |
|---|---|---|---|---|---|---|
| burst | push | 2089 | 1ns | 64ns | 64ns | 100ns |
| burst | pop | 226 | 1ns | 1ns | 64ns | 100ns |
| real | push | 2089 | 64ns | 256ns | 256ns | 19.5μs |
| real | pop | 6821 | 64ns | 256ns | 256ns | 32.4μs |

## Debug build

| Speed | Op | count | p50 | p90 | p99 | max |
|---|---|---|---|---|---|---|
| burst | push | 2089 | 128ns | 256ns | 256ns | 4.1μs |
| burst | pop | 1816 | 256ns | 512ns | 2.0μs | 29.7μs |
| real | push | 2089 | 512ns | 512ns | 1.0μs | 6.3μs |
| real | pop | 6880 | 2.0μs | 4.1μs | 8.2μs | 199μs |
