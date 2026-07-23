#!/usr/bin/env bash
# Generates Go gRPC bindings from engine/proto/paper_trader.proto into
# genproto/ (gitignored, regenerated on demand -- mirrors the C++ side's
# CMake custom command and the same idea the old Python backend used).
# Run from backend/.
#
# Requires protoc on PATH -- a standalone prerequisite, same as Go itself,
# not something borrowed from the engine's vcpkg build. Install from
# https://github.com/protocolbuffers/protobuf/releases (or your OS package
# manager) and add it to PATH.
set -euo pipefail

cd "$(dirname "$0")/.."

if ! command -v protoc >/dev/null 2>&1; then
  echo "protoc not found on PATH -- install it from https://github.com/protocolbuffers/protobuf/releases" >&2
  exit 1
fi

mkdir -p .tools genproto

go build -o .tools/protoc-gen-go.exe google.golang.org/protobuf/cmd/protoc-gen-go
go build -o .tools/protoc-gen-go-grpc.exe google.golang.org/grpc/cmd/protoc-gen-go-grpc

protoc \
  -I "../engine/proto" \
  --plugin=protoc-gen-go=.tools/protoc-gen-go.exe \
  --plugin=protoc-gen-go-grpc=.tools/protoc-gen-go-grpc.exe \
  --go_out=genproto --go_opt=paths=source_relative \
  --go_opt=Mpaper_trader.proto=papertrader/backend/genproto \
  --go-grpc_out=genproto --go-grpc_opt=paths=source_relative \
  --go-grpc_opt=Mpaper_trader.proto=papertrader/backend/genproto \
  ../engine/proto/paper_trader.proto

echo "generated Go gRPC bindings in genproto/"
