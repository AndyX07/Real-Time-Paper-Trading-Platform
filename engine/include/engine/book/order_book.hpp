#pragma once

#include <vector>
#include <span>
#include <string>
#include "engine/book/price_level.hpp"

enum class BookSide : uint8_t { Bid, Ask };

class OrderBookSide {
public:
    explicit OrderBookSide(BookSide side) : side_(side) {}
    void applyDelta(Price price, Quantity newQuantity);
    void clear();
    const PriceLevel& top() const;
    std::span<const PriceLevel> depth(size_t n) const;

private:
    bool better(Price a, Price b) const;

    BookSide side_;
    // sorted vector for price levels
    // better than map for small n
    std::vector<PriceLevel> levels_;
};

struct OrderBook {
    std::string symbol;
    OrderBookSide bids{BookSide::Bid};
    OrderBookSide asks{BookSide::Ask};
    uint64_t lastSeq{0};
    uint32_t lastCheckSum{0};
};