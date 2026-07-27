#pragma once

#include <cstdint>
#include <functional>
#include <map>
#include <string>
#include <unordered_map>
#include <utility>

#include "engine/book/order_book.hpp"
#include "engine/common/price_quantity.hpp"
#include "engine/matching/paper_order.hpp"

class MatchingEngine {
public:
    using FillCallback = std::function<void(const Fill&)>;
    MatchingEngine(std::string symbol, const OrderBook& book, FillCallback onFill);
    OrderResult placeOrder(uint64_t orderId, BookSide side, OrderType type, Price price, Quantity size);
    OrderResult cancelOrder(uint64_t orderId);
    void onBookDelta(BookSide side, Price price, Quantity newQuantity);

private:
    using LevelKey = std::pair<BookSide, Price>;

    void emitFill(uint64_t orderId, Price price, Quantity size);
    bool anyRestingOrderAt(const LevelKey& key) const;

    std::string symbol_;
    const OrderBook& book_;
    FillCallback onFill_;

    std::unordered_map<uint64_t, PaperOrder> restingOrders_;
    std::map<LevelKey, Quantity> lastObservedSize_;
};
