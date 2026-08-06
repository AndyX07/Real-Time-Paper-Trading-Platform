#!/usr/bin/env bash

# Usage: scripts/record_kraken_feed.sh [symbol] [duration_seconds] [out_path]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

SYMBOL="${1:-BTC/USD}"
DURATION="${2:-300}"
OUT="${3:-$REPO_ROOT/recordings/$(date +%Y%m%d-%H%M%S)-${SYMBOL//\//_}.ptrec}"

RECORDER="$REPO_ROOT/engine/out/build/debug-local/tools/replay/kraken_recorder.exe"
if [ ! -f "$RECORDER" ]; then
    echo "record_kraken_feed: $RECORDER not found -- build the engine first (see engine/CMakePresets.json)" >&2
    exit 1
fi

mkdir -p "$(dirname "$OUT")"
exec "$RECORDER" --symbol "$SYMBOL" --out "$OUT" --duration "$DURATION"
