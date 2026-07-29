#include <cstdint>
#include <cstdlib>
#include <iostream>
#include <memory>
#include <random>
#include <string>

#include <grpcpp/grpcpp.h>

#include "engine/control/control_server.hpp"
#include "engine/ipc/shared_memory_manager.hpp"
#include "engine/market_data/kraken_asset_pairs.hpp"
#include "engine/symbol/symbol_registry.hpp"

namespace {

std::string controlPlaneAddress() {
    const char* port = std::getenv("ENGINE_GRPC_PORT");
    return "127.0.0.1:" + std::string(port ? port : "50051");
}

uint64_t generateInstanceId() {
    std::random_device rd;
    std::mt19937_64 gen{rd()};
    std::uniform_int_distribution<uint64_t> dist;
    return dist(gen);
}

}

int main() {
    SharedMemoryManager sharedMemory;
    SymbolRegistry symbolRegistry{sharedMemory, fetchKrakenAssetPairPrecision};
    ControlServiceImpl service{symbolRegistry, generateInstanceId()};

    std::string address = controlPlaneAddress();
    grpc::ServerBuilder builder;
    builder.AddListeningPort(address, grpc::InsecureServerCredentials());
    builder.RegisterService(&service);

    std::unique_ptr<grpc::Server> server{builder.BuildAndStart()};
    if (!server) {
        std::cerr << "engine: failed to start gRPC server on " << address << "\n";
        return 1;
    }

    std::cout << "engine: listening on " << address << "\n";
    server->Wait();
    return 0;
}
