#pragma once

#include <functional>
#include <memory>
#include <mutex>
#include <optional>
#include <string>
#include <string_view>
#include <thread>
#include <unordered_map>

#include "engine/book/order_book.hpp"
#include "engine/ipc/shared_memory_manager.hpp"
#include "engine/market_data/kraken_asset_pairs.hpp"
#include "engine/market_data/kraken_book_client.hpp"

struct SubscribeResult {
    bool ok;
    std::string reason; // empty when ok
};

class SymbolRegistry {
public:
    using PrecisionLookup = std::function<std::optional<SymbolPrecision>(std::string_view symbol)>;

    SymbolRegistry(SharedMemoryManager& sharedMemory, PrecisionLookup precisionLookup);

    ~SymbolRegistry();

    SymbolRegistry(const SymbolRegistry&) = delete;
    SymbolRegistry& operator=(const SymbolRegistry&) = delete;

    SubscribeResult subscribe(std::string_view symbol);

    SubscribeResult unsubscribe(std::string_view symbol);

private:
    struct Entry {
        int refcount = 0;
        SymbolSlot* slot = nullptr;
        std::unique_ptr<OrderBook> book;
        std::unique_ptr<KrakenBookClient> client;
        std::thread thread;
    };

    std::mutex mutex_;
    std::unordered_map<std::string, Entry> entries_;
    SharedMemoryManager& sharedMemory_;
    PrecisionLookup precisionLookup_;
};
