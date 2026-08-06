#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

ENGINE="$REPO_ROOT/engine/out/build/debug-local/engine.exe"
if [ ! -f "$ENGINE" ]; then
    echo "run_dev: $ENGINE not found -- build the engine first (see engine/CMakePresets.json)" >&2
    exit 1
fi

trap 'kill $(jobs -p) 2>/dev/null || true' EXIT

"$ENGINE" &
( cd "$REPO_ROOT/backend" && go run ./cmd/server ) &
( cd "$REPO_ROOT/frontend" && npm run dev ) &

wait -n
