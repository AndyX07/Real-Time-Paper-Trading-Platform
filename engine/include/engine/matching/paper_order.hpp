#pragma once

#include <cstdint>
#include <string>

#include "engine/book/order_book.hpp"
#include "engine/common/price_quantity.hpp"

// for market orders, walks the orderbook until either the order's size is used up
// or the depth runs out, what is left of the order is cancelled

// for limit orders, it does the same walk as market orders, but only fills until the limit price
// if there's leftover size, the order rests with a queueAheadSize which is how much volume is at that price level before the order
// it waits for queueAheadSize volume of size reductions at that price level before it starts filling

enum class OrderType : uint8_t { Market, Limit };

struct PaperOrder {
    uint64_t orderId;
    std::string symbol;
    BookSide side;
    OrderType type;
    Price price; // ignored for market orders
    Quantity remainingSize;
    Quantity queueAheadSize; // size of price level when order submitted
    uint64_t insertedAtSeq;
};

struct Fill {
    uint64_t orderId;
    Price price;
    Quantity size;
    uint64_t timestamp;
};

struct OrderResult {
    bool accepted;
    uint64_t orderId; // set if accepted
    std::string rejectReason; // set if rejected
    // how much quantity matched synchronously
    Quantity filledSize{};
};
