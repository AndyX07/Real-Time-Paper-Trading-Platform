#pragma once

#include <atomic>
#include <chrono>
#include <condition_variable>
#include <deque>
#include <functional>
#include <future>
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
#include "engine/matching/matching_engine.hpp"
#include "engine/matching/paper_order.hpp"

struct SubscribeResult {
    bool ok;
    std::string reason; // empty when ok
};

class SymbolRegistry {
public:
    using PrecisionLookup = std::function<std::optional<SymbolPrecision>(std::string_view symbol)>;

    SymbolRegistry(SharedMemoryManager& sharedMemory, PrecisionLookup precisionLookup,
                   std::optional<std::string> replaySessionPath = std::nullopt);

    ~SymbolRegistry();

    SymbolRegistry(const SymbolRegistry&) = delete;
    SymbolRegistry& operator=(const SymbolRegistry&) = delete;

    SubscribeResult subscribe(std::string_view symbol);

    SubscribeResult unsubscribe(std::string_view symbol);

    OrderResult placeOrder(std::string_view symbol, BookSide side, OrderType type, Price price, Quantity size);
    OrderResult cancelOrder(uint64_t engineOrderId);

    bool waitForFill(Fill& out, std::chrono::milliseconds timeout);

private:
    struct OrderRequest {
        enum class Kind { Place, Cancel } kind;
        BookSide side{};
        OrderType type{};
        Price price{};
        Quantity size{};
        uint64_t orderId{};
        std::promise<OrderResult> result;
    };


    struct OrderInbox {
        std::mutex mutex;
        std::deque<OrderRequest> requests;
    };

    struct Entry {
        int refcount = 0;
        SymbolSlot* slot = nullptr;
        std::unique_ptr<OrderBook> book;
        std::unique_ptr<KrakenBookClient> client;
        std::unique_ptr<MatchingEngine> matchingEngine;
        std::unique_ptr<OrderInbox> inbox;
        std::thread thread;
    };

    void drainInbox(OrderInbox* inbox, MatchingEngine* engine);
    void pushFill(const Fill& fill);

    std::mutex mutex_;
    std::unordered_map<std::string, std::shared_ptr<Entry>> entries_;
    std::unordered_map<uint64_t, std::string> orderIdToSymbol_;
    std::atomic<uint64_t> nextOrderId_{1};
    SharedMemoryManager& sharedMemory_;
    PrecisionLookup precisionLookup_;
    std::optional<std::string> replaySessionPath_;

    std::mutex fillMutex_;
    std::condition_variable fillCv_;
    std::deque<Fill> fillQueue_;
};
