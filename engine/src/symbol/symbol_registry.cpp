#include "engine/symbol/symbol_registry.hpp"

#include <algorithm>
#include <chrono>
#include <cstring>
#include <span>
#include <stdexcept>

#include "engine/common/price_quantity.hpp"
#include "engine/ipc/ring_buffer.hpp"
#include "engine/ipc/snapshot_slot.hpp"

namespace {

void setSnapshotSymbol(BookSnapshot& snapshot, std::string_view symbol) {
    if (symbol.size() >= sizeof(snapshot.symbol)) {
        throw std::invalid_argument("BookSnapshot: symbol too long for fixed-width field");
    }
    std::memset(snapshot.symbol, 0, sizeof(snapshot.symbol));
    std::memcpy(snapshot.symbol, symbol.data(), symbol.size());
}

}

SymbolRegistry::SymbolRegistry(SharedMemoryManager& sharedMemory, PrecisionLookup precisionLookup)
    : sharedMemory_(sharedMemory), precisionLookup_(std::move(precisionLookup)) {}

SymbolRegistry::~SymbolRegistry() {
    std::lock_guard<std::mutex> lock(mutex_);

    for (auto& [symbol, entry] : entries_) {
        entry.client->stop();
    }
    for (auto& [symbol, entry] : entries_) {
        entry.thread.join();
        sharedMemory_.releaseSlot(entry.slot);
    }
}

SubscribeResult SymbolRegistry::subscribe(std::string_view symbol) {
    std::string key(symbol);

    {
        std::lock_guard<std::mutex> lock(mutex_);
        auto it = entries_.find(key);
        if (it != entries_.end()) {
            ++it->second.refcount;
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

    std::lock_guard<std::mutex> lock(mutex_);

    // Re-check: another thread may have completed a 0->1 subscribe for
    // this exact symbol while the lookup above ran without the lock
    // held.
    auto it = entries_.find(key);
    if (it != entries_.end()) {
        ++it->second.refcount;
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

    Entry entry;
    entry.refcount = 1;
    entry.slot = slot;
    entry.book = std::make_unique<OrderBook>();
    entry.book->symbol = key;

    KrakenBookClient::BookDeltaCallback onDelta =
        [slot, key](BookSide side, Price price, Quantity quantity, uint64_t seq) {
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
            slot->deltaQueue.tryPush(delta);
        };

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

    entry.client = std::make_unique<KrakenBookClient>(key, *entry.book, std::move(onDelta), std::move(onSnapshot),
                                                       precision->pairDecimals, precision->lotDecimals);
    KrakenBookClient* clientPtr = entry.client.get();
    entry.thread = std::thread([clientPtr] { clientPtr->run(); });

    entries_.emplace(std::move(key), std::move(entry));
    return {true, ""};
}

SubscribeResult SymbolRegistry::unsubscribe(std::string_view symbol) {
    Entry extracted;
    bool shouldTeardown = false;

    {
        std::lock_guard<std::mutex> lock(mutex_);
        std::string key(symbol);
        auto it = entries_.find(key);
        if (it == entries_.end()) {
            return {false, "symbol not subscribed"};
        }

        if (--it->second.refcount > 0) {
            return {true, ""};
        }

        it->second.client->stop();
        extracted = std::move(it->second);
        entries_.erase(it);
        shouldTeardown = true;
    }

    if (shouldTeardown) {
        extracted.thread.join();
        sharedMemory_.releaseSlot(extracted.slot);
    }

    return {true, ""};
}
