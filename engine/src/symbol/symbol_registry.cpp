#include "engine/symbol/symbol_registry.hpp"

#include <algorithm>
#include <chrono>
#include <cstring>
#include <span>
#include <stdexcept>

#include "engine/common/price_quantity.hpp"
#include "engine/ipc/ring_buffer.hpp"
#include "engine/ipc/snapshot_slot.hpp"
#include "engine/observability/histogram.hpp"

namespace {

constexpr std::chrono::seconds kOrderRequestTimeout{30};

void setSnapshotSymbol(BookSnapshot& snapshot, std::string_view symbol) {
    if (symbol.size() >= sizeof(snapshot.symbol)) {
        throw std::invalid_argument("BookSnapshot: symbol too long for fixed-width field");
    }
    std::memset(snapshot.symbol, 0, sizeof(snapshot.symbol));
    std::memcpy(snapshot.symbol, symbol.data(), symbol.size());
}

}

SymbolRegistry::SymbolRegistry(SharedMemoryManager& sharedMemory, PrecisionLookup precisionLookup)
    : sharedMemory_{sharedMemory}, precisionLookup_{std::move(precisionLookup)} {}

SymbolRegistry::~SymbolRegistry() {
    std::lock_guard<std::mutex> lock{mutex_};

    for (auto& [symbol, entry] : entries_) {
        entry->client->stop();
    }
    for (auto& [symbol, entry] : entries_) {
        entry->thread.join();
        sharedMemory_.releaseSlot(entry->slot);
    }
}

SubscribeResult SymbolRegistry::subscribe(std::string_view symbol) {
    std::string key{symbol};

    {
        std::lock_guard<std::mutex> lock{mutex_};
        auto it = entries_.find(key);
        if (it != entries_.end()) {
            ++it->second->refcount;
            return {true, ""};
        }
    }

    auto precision = precisionLookup_(symbol);
    if (!precision) {
        return {false, "could not determine symbol precision"};
    }
    if (!priceScaleSupports(precision->pairDecimals) || !quantityScaleSupports(precision->lotDecimals)) {
        return {false, "symbol requires more precision than PRICE_SCALE/QUANTITY_SCALE supports"};
    }

    std::lock_guard<std::mutex> lock{mutex_};

    // Re-check: another thread may have completed a 0->1 subscribe for
    // this exact symbol while the lookup above ran without the lock
    // held.
    auto it = entries_.find(key);
    if (it != entries_.end()) {
        ++it->second->refcount;
        return {true, ""};
    }

    SymbolSlot* slot = nullptr;
    try {
        slot = sharedMemory_.claimSlot(key);
    } catch (const std::exception& e) {
        return {false, e.what()};
    }
    if (slot == nullptr) {
        return {false, "symbol slot pool exhausted"};
    }

    auto entry = std::make_shared<Entry>();
    entry->refcount = 1;
    entry->slot = slot;
    entry->book = std::make_unique<OrderBook>();
    entry->book->symbol = key;
    entry->inbox = std::make_unique<OrderInbox>();

    MatchingEngine::FillCallback onFill = [this](const Fill& fill) { pushFill(fill); };
    entry->matchingEngine = std::make_unique<MatchingEngine>(key, *entry->book, std::move(onFill));

    OrderInbox* inboxPtr = entry->inbox.get();
    MatchingEngine* enginePtr = entry->matchingEngine.get();

    KrakenBookClient::BookDeltaCallback onDelta =
        [slot, key, enginePtr](BookSide side, Price price, Quantity quantity, uint64_t seq) {
            BookDelta delta{};
            setSymbol(delta, key);
            delta.seq = seq;
            delta.engineTsNanos = static_cast<uint64_t>(
                std::chrono::duration_cast<std::chrono::nanoseconds>(
                    std::chrono::system_clock::now().time_since_epoch())
                    .count());
            delta.side = toWireSide(side);
            delta.price = price;
            delta.size = quantity;
            {
                ScopedLatencyTimer timer{HistogramRegistry::instance().get("ipc.ring_push")};
                slot->deltaQueue.tryPush(delta);
            }
            enginePtr->onBookDelta(side, price, quantity);
        };

    // Fires on every WS message, including heartbeats -- onDelta/onSnapshot
    // alone can't guarantee a bounded drain cadence, since a genuinely
    // quiet book (or a symbol still connecting for the first time) may
    // never produce either for longer than an order request is willing to
    // wait (see kOrderRequestTimeout).
    KrakenBookClient::TickCallback onTick = [this, inboxPtr, enginePtr] { drainInbox(inboxPtr, enginePtr); };

    KrakenBookClient::BookSnapshotCallback onSnapshot =
        [slot, key](std::span<const PriceLevel> bids, std::span<const PriceLevel> asks, uint64_t seq) {
            BookSnapshot snapshot{};
            setSnapshotSymbol(snapshot, key);
            snapshot.seq = seq;
            snapshot.engineTsNanos = static_cast<uint64_t>(
                std::chrono::duration_cast<std::chrono::nanoseconds>(
                    std::chrono::system_clock::now().time_since_epoch())
                    .count());
            snapshot.numBidLevels = static_cast<uint16_t>(std::min(bids.size(), BOOK_DEPTH));
            snapshot.numAskLevels = static_cast<uint16_t>(std::min(asks.size(), BOOK_DEPTH));
            for (size_t i = 0; i < snapshot.numBidLevels; ++i) {
                snapshot.bids[i] = bids[i];
            }
            for (size_t i = 0; i < snapshot.numAskLevels; ++i) {
                snapshot.asks[i] = asks[i];
            }
            slot->snapshotSlot.write(snapshot);
        };

    entry->client = std::make_unique<KrakenBookClient>(key, *entry->book, std::move(onDelta), std::move(onSnapshot),
                                                        precision->pairDecimals, precision->lotDecimals,
                                                        std::move(onTick));
    KrakenBookClient* clientPtr = entry->client.get();
    entry->thread = std::thread([clientPtr] { clientPtr->run(); });

    entries_.emplace(std::move(key), std::move(entry));
    return {true, ""};
}

SubscribeResult SymbolRegistry::unsubscribe(std::string_view symbol) {
    std::shared_ptr<Entry> extracted;
    bool shouldTeardown = false;

    {
        std::lock_guard<std::mutex> lock{mutex_};
        std::string key{symbol};
        auto it = entries_.find(key);
        if (it == entries_.end()) {
            return {false, "symbol not subscribed"};
        }

        if (--it->second->refcount > 0) {
            return {true, ""};
        }

        it->second->client->stop();
        extracted = it->second;
        entries_.erase(it);
        shouldTeardown = true;
    }

    if (shouldTeardown) {
        extracted->thread.join();
        sharedMemory_.releaseSlot(extracted->slot);
    }

    return {true, ""};
}

OrderResult SymbolRegistry::placeOrder(std::string_view symbol, BookSide side, OrderType type, Price price, Quantity size) {
    std::string key{symbol};
    std::shared_ptr<Entry> entry;
    {
        std::lock_guard<std::mutex> lock{mutex_};
        auto it = entries_.find(key);
        if (it == entries_.end()) {
            return {false, 0, "symbol not subscribed"};
        }
        entry = it->second;
    }

    uint64_t orderId = nextOrderId_.fetch_add(1, std::memory_order_relaxed);

    OrderRequest request;
    request.kind = OrderRequest::Kind::Place;
    request.side = side;
    request.type = type;
    request.price = price;
    request.size = size;
    request.orderId = orderId;
    std::future<OrderResult> future = request.result.get_future();

    {
        std::lock_guard<std::mutex> lock{entry->inbox->mutex};
        entry->inbox->requests.push_back(std::move(request));
    }

    if (future.wait_for(kOrderRequestTimeout) != std::future_status::ready) {
        return {false, 0, "order request timed out (symbol thread unresponsive)"};
    }

    OrderResult result = future.get();
    if (result.accepted) {
        std::lock_guard<std::mutex> lock{mutex_};
        orderIdToSymbol_[orderId] = key;
    }
    return result;
}

OrderResult SymbolRegistry::cancelOrder(uint64_t engineOrderId) {
    std::shared_ptr<Entry> entry;
    {
        std::lock_guard<std::mutex> lock{mutex_};
        auto idIt = orderIdToSymbol_.find(engineOrderId);
        if (idIt == orderIdToSymbol_.end()) {
            return {false, 0, "order not found"};
        }
        auto it = entries_.find(idIt->second);
        if (it == entries_.end()) {
            return {false, 0, "order not found"};
        }
        entry = it->second;
    }

    OrderRequest request;
    request.kind = OrderRequest::Kind::Cancel;
    request.orderId = engineOrderId;
    std::future<OrderResult> future = request.result.get_future();

    {
        std::lock_guard<std::mutex> lock{entry->inbox->mutex};
        entry->inbox->requests.push_back(std::move(request));
    }

    if (future.wait_for(kOrderRequestTimeout) != std::future_status::ready) {
        return {false, 0, "cancel request timed out (symbol thread unresponsive)"};
    }

    OrderResult result = future.get();
    if (result.accepted) {
        std::lock_guard<std::mutex> lock{mutex_};
        orderIdToSymbol_.erase(engineOrderId);
    }
    return result;
}

void SymbolRegistry::drainInbox(OrderInbox* inbox, MatchingEngine* engine) {
    std::deque<OrderRequest> pending;
    {
        std::lock_guard<std::mutex> lock{inbox->mutex};
        pending.swap(inbox->requests);
    }

    for (auto& request : pending) {
        OrderResult result = (request.kind == OrderRequest::Kind::Place)
                                  ? engine->placeOrder(request.orderId, request.side, request.type, request.price,
                                                        request.size)
                                  : engine->cancelOrder(request.orderId);
        request.result.set_value(result);
    }
}

void SymbolRegistry::pushFill(const Fill& fill) {
    std::lock_guard<std::mutex> lock{fillMutex_};
    fillQueue_.push_back(fill);
    fillCv_.notify_one();
}

bool SymbolRegistry::waitForFill(Fill& out, std::chrono::milliseconds timeout) {
    std::unique_lock<std::mutex> lock{fillMutex_};
    if (!fillCv_.wait_for(lock, timeout, [this] { return !fillQueue_.empty(); })) {
        return false;
    }
    out = fillQueue_.front();
    fillQueue_.pop_front();
    return true;
}
