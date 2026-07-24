#include "engine/book/order_book.hpp"

#include <algorithm>
#include <stdexcept>

bool OrderBookSide::better(Price a, Price b) const {
    return side_ == BookSide::Bid ? a > b : a < b;
}

std::optional<Price> OrderBookSide::applyDelta(Price price, Quantity newQuantity) {
    auto it = std::lower_bound(
        levels_.begin(), levels_.end(), price,
        [this](const PriceLevel& level, Price target) { return better(level.price, target); });

    bool foundExisting = it != levels_.end() && it->price == price;

    if (newQuantity.ticks == 0) {
        if (foundExisting) {
            levels_.erase(it);
        }
        return std::nullopt;
    }

    if (foundExisting) {
        it->quantity = newQuantity;
    } else {
        levels_.insert(it, PriceLevel{price, newQuantity});
    }

    if (levels_.size() > BOOK_DEPTH) {
        Price evicted = levels_.back().price;
        levels_.resize(BOOK_DEPTH);
        return evicted;
    }
    return std::nullopt;
}

void OrderBookSide::clear() {
    levels_.clear();
}

const PriceLevel& OrderBookSide::top() const {
    if (levels_.empty()) {
        throw std::out_of_range("OrderBookSide::top: side has no levels");
    }
    return levels_[0];
}

std::span<const PriceLevel> OrderBookSide::depth(size_t n) const {
    return std::span<const PriceLevel>{levels_.data(), std::min(n, levels_.size())};
}