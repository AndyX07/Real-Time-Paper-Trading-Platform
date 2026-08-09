// Test-only executable, never shipped: a thin, fixed-configuration copy of
// main.cpp's composition root, used by the Go backend's cross-process
// integration test (backend/internal/integration/engine_cross_process_test.go).
//
// Everything is hardcoded rather than read from the environment or argv,
// on purpose -- this exists specifically so that test doesn't need to teach
// the real engine.exe entrypoint about test-only configuration:
//   - shared-memory segment name: "paper_trader_test_fixed" (never collides
//     with a developer's real dev engine, which always uses the production
//     default name)
//   - gRPC control-plane address: 127.0.0.1:57050 (never the production
//     default port 50051)
//   - market data: replayed from the committed golden_session.ptrec fixture
//     instead of a live Kraken connection -- no network dependency
//   - precision: hardcoded to match the fixture's own recorded precision,
//     instead of a real Kraken REST call
#include <iostream>
#include <memory>
#include <optional>
#include <string>

#include <grpcpp/grpcpp.h>

#include "engine/control/control_server.hpp"
#include "engine/ipc/shared_memory_manager.hpp"
#include "engine/market_data/kraken_asset_pairs.hpp"
#include "engine/symbol/symbol_registry.hpp"

namespace {

constexpr const char* kSegmentName = "paper_trader_test_fixed";
constexpr const char* kControlAddress = "127.0.0.1:57050";
constexpr uint64_t kFixedInstanceId = 1;

std::optional<SymbolPrecision> fixedPrecisionLookup(std::string_view symbol) {
    if (symbol != "BTC/USD") {
        return std::nullopt;
    }
    // Matches the defaults kraken_replayer already assumes when it produced
    // this same fixture -- see engine/tools/replay/replayer.cpp's Args.
    return SymbolPrecision{.pairDecimals = 1, .lotDecimals = 8};
}

}

int main() {
    SharedMemoryManager sharedMemory{kSegmentName};
    SymbolRegistry symbolRegistry{sharedMemory, fixedPrecisionLookup, std::string{TEST_ENGINE_HARNESS_FIXTURE_PATH}};
    ControlServiceImpl service{symbolRegistry, kFixedInstanceId};

    grpc::ServerBuilder builder;
    builder.AddListeningPort(kControlAddress, grpc::InsecureServerCredentials());
    builder.RegisterService(&service);

    std::unique_ptr<grpc::Server> server{builder.BuildAndStart()};
    if (!server) {
        std::cerr << "test_engine_harness: failed to start gRPC server on " << kControlAddress << "\n";
        return 1;
    }

    std::cout << "test_engine_harness: listening on " << kControlAddress
              << ", segment=" << kSegmentName << ", replaying " << TEST_ENGINE_HARNESS_FIXTURE_PATH << "\n";
    server->Wait();
    return 0;
}
