#include "engine/matching/matching_engine.hpp"

#include <algorithm>
#include <chrono>
#include <vector>

namespace {

uint64_t nowNanos() {
    return static_cast<uint64_t>(
        std::chrono::duration_cast<std::chrono::nanoseconds>(std::chrono::system_clock::now().time_since_epoch())
            .count());
}

}

MatchingEngine::MatchingEngine(std::string symbol, const OrderBook& book, FillCallback onFill)
    : symbol_{std::move(symbol)}, book_{book}, onFill_{std::move(onFill)} {}

void MatchingEngine::emitFill(uint64_t orderId, Price price, Quantity size) {
    if (onFill_) {
        onFill_(Fill{orderId, price, size, nowNanos()});
    }
}

bool MatchingEngine::anyRestingOrderAt(const LevelKey& key) const {
    for (const auto& [id, order] : restingOrders_) {
        if (order.side == key.first && order.price == key.second) {
            return true;
        }
    }
    return false;
}

OrderResult MatchingEngine::placeOrder(uint64_t orderId, BookSide side, OrderType type, Price price, Quantity size) {
    if (size.ticks <= 0) {
        return {false, 0, "size must be positive"};
    }
    if (type == OrderType::Limit && price.ticks <= 0) {
        return {false, 0, "limit price must be positive"};
    }
    const OrderBookSide& opposite = (side == BookSide::Bid) ? book_.asks : book_.bids;

    Quantity remaining = size;
    for (const PriceLevel& level : opposite.depth(BOOK_DEPTH)) {
        if (remaining.ticks <= 0) {
            break;
        }
        if (type == OrderType::Limit) {
            bool crossed = (side == BookSide::Bid) ? (level.price <= price) : (level.price >= price);
            if (!crossed) {
                break;
            }
        }
        Quantity fillSize = std::min(remaining, level.quantity);
        emitFill(orderId, level.price, fillSize);
        remaining -= fillSize;
    }

    if (remaining.ticks > 0 && type == OrderType::Limit) {
        PaperOrder order{};
        order.orderId = orderId;
        order.symbol = symbol_;
        order.side = side;
        order.type = type;
        order.price = price;
        order.remainingSize = remaining;

        const OrderBookSide& ownSide = (side == BookSide::Bid) ? book_.bids : book_.asks;
        order.queueAheadSize = ownSide.sizeAtPrice(price);
        order.insertedAtSeq = book_.lastSeq;

        LevelKey key{side, price};
        lastObservedSize_.try_emplace(key, order.queueAheadSize);
        restingOrders_.emplace(orderId, std::move(order));
    }

    return {true, orderId, ""};
}

OrderResult MatchingEngine::cancelOrder(uint64_t orderId) {
    auto it = restingOrders_.find(orderId);
    if (it == restingOrders_.end()) {
        return {false, 0, "order not found"};
    }

    LevelKey key{it->second.side, it->second.price};
    restingOrders_.erase(it);
    if (!anyRestingOrderAt(key)) {
        lastObservedSize_.erase(key);
    }
    return {true, orderId, ""};
}

void MatchingEngine::onBookDelta(BookSide side, Price price, Quantity newQuantity) {
    LevelKey key{side, price};
    auto trackedIt = lastObservedSize_.find(key);
    if (trackedIt == lastObservedSize_.end()) {
        return;
    }

    Quantity previous = trackedIt->second;
    trackedIt->second = newQuantity;

    if (newQuantity.ticks >= previous.ticks) {
        return;
    }
    Quantity reduction = previous - newQuantity;

    std::vector<uint64_t> fullyFilled;
    for (auto& [orderId, order] : restingOrders_) {
        if (order.side != side || order.price != price) {
            continue;
        }

        Quantity absorbed = std::min(order.queueAheadSize, reduction);
        order.queueAheadSize -= absorbed;
        Quantity overflow = reduction - absorbed;

        if (overflow.ticks > 0 && order.remainingSize.ticks > 0) {
            Quantity fillSize = std::min(overflow, order.remainingSize);
            emitFill(orderId, order.price, fillSize);
            order.remainingSize -= fillSize;
            if (order.remainingSize.ticks == 0) {
                fullyFilled.push_back(orderId);
            }
        }
    }

    for (uint64_t id : fullyFilled) {
        restingOrders_.erase(id);
    }
    if (!anyRestingOrderAt(key)) {
        lastObservedSize_.erase(key);
    }
}
