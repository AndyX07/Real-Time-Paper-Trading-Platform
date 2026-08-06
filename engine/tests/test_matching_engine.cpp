#include <catch2/catch_test_macros.hpp>

#include <random>
#include <vector>

#include "engine/matching/matching_engine.hpp"

namespace {

// Captures every Fill emitted during a test, in emission order.
struct FillRecorder {
    std::vector<Fill> fills;
    MatchingEngine::FillCallback callback() {
        return [this](const Fill& f) { fills.push_back(f); };
    }
};

}

TEST_CASE("placeOrder rejects non-positive size", "[matching_engine]") {
    OrderBook book;
    FillRecorder recorder;
    MatchingEngine engine{"BTC/USD", book, recorder.callback()};

    auto result = engine.placeOrder(1, BookSide::Bid, OrderType::Market, Price{0}, Quantity{0});
    CHECK_FALSE(result.accepted);
    CHECK(result.rejectReason == "size must be positive");

    auto negative = engine.placeOrder(2, BookSide::Bid, OrderType::Market, Price{0}, Quantity{-1});
    CHECK_FALSE(negative.accepted);
    CHECK(negative.rejectReason == "size must be positive");
}

TEST_CASE("placeOrder rejects non-positive limit price", "[matching_engine]") {
    OrderBook book;
    FillRecorder recorder;
    MatchingEngine engine{"BTC/USD", book, recorder.callback()};

    auto result = engine.placeOrder(1, BookSide::Bid, OrderType::Limit, Price{0}, Quantity::fromDecimalString("1.0"));
    CHECK_FALSE(result.accepted);
    CHECK(result.rejectReason == "limit price must be positive");
}

TEST_CASE("cancelOrder on an unknown id is rejected", "[matching_engine]") {
    OrderBook book;
    FillRecorder recorder;
    MatchingEngine engine{"BTC/USD", book, recorder.callback()};

    auto result = engine.cancelOrder(999);
    CHECK_FALSE(result.accepted);
    CHECK(result.rejectReason == "order not found");
}

TEST_CASE("market order exhausting book depth fills across every level and discards the remainder", "[matching_engine]") {
    OrderBook book;
    book.asks.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("2.0"));
    book.asks.applyDelta(Price::fromDecimalString("101.0"), Quantity::fromDecimalString("3.0"));
    book.asks.applyDelta(Price::fromDecimalString("102.0"), Quantity::fromDecimalString("1.0"));

    FillRecorder recorder;
    MatchingEngine engine{"BTC/USD", book, recorder.callback()};

    auto result = engine.placeOrder(1, BookSide::Bid, OrderType::Market, Price{0}, Quantity::fromDecimalString("10.0"));

    REQUIRE(result.accepted);
    CHECK(result.filledSize == Quantity::fromDecimalString("6.0")); // 2 + 3 + 1, not the requested 10
    REQUIRE(recorder.fills.size() == 3);
    CHECK(recorder.fills[0].price == Price::fromDecimalString("100.0"));
    CHECK(recorder.fills[0].size == Quantity::fromDecimalString("2.0"));
    CHECK(recorder.fills[1].price == Price::fromDecimalString("101.0"));
    CHECK(recorder.fills[1].size == Quantity::fromDecimalString("3.0"));
    CHECK(recorder.fills[2].price == Price::fromDecimalString("102.0"));
    CHECK(recorder.fills[2].size == Quantity::fromDecimalString("1.0"));

    // Market orders never rest -- the unfilled remainder is simply gone.
    CHECK_FALSE(engine.cancelOrder(1).accepted);
}

TEST_CASE("limit order partial-fills across multiple price levels and rests for the remainder", "[matching_engine]") {
    OrderBook book;
    book.asks.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("2.0"));
    book.asks.applyDelta(Price::fromDecimalString("101.0"), Quantity::fromDecimalString("3.0"));
    book.asks.applyDelta(Price::fromDecimalString("102.0"), Quantity::fromDecimalString("5.0")); // outside the limit

    FillRecorder recorder;
    MatchingEngine engine{"BTC/USD", book, recorder.callback()};

    auto result = engine.placeOrder(1, BookSide::Bid, OrderType::Limit, Price::fromDecimalString("101.5"),
                                     Quantity::fromDecimalString("8.0"));

    REQUIRE(result.accepted);
    CHECK(result.filledSize == Quantity::fromDecimalString("5.0")); // 2 + 3, the 102 level never crosses
    REQUIRE(recorder.fills.size() == 2);
    CHECK(recorder.fills[0].price == Price::fromDecimalString("100.0"));
    CHECK(recorder.fills[1].price == Price::fromDecimalString("101.0"));

    // 3.0 remaining -- rests, proven by a subsequent cancel succeeding.
    CHECK(engine.cancelOrder(1).accepted);
}

TEST_CASE("limit order priced to not cross the book rests fully unfilled", "[matching_engine]") {
    OrderBook book;
    book.asks.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("5.0"));

    FillRecorder recorder;
    MatchingEngine engine{"BTC/USD", book, recorder.callback()};

    auto result =
        engine.placeOrder(1, BookSide::Bid, OrderType::Limit, Price::fromDecimalString("99.0"), Quantity::fromDecimalString("3.0"));

    REQUIRE(result.accepted);
    CHECK(result.filledSize == Quantity{0});
    CHECK(recorder.fills.empty());
    CHECK(engine.cancelOrder(1).accepted);
}

TEST_CASE("a resting order does not fill while cumulative reductions stay under its own queueAheadSize", "[matching_engine]") {
    OrderBook book;
    book.bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("10.0"));

    FillRecorder recorder;
    MatchingEngine engine{"BTC/USD", book, recorder.callback()};

    auto result = engine.placeOrder(1, BookSide::Bid, OrderType::Limit, Price::fromDecimalString("100.0"),
                                     Quantity::fromDecimalString("5.0"));
    REQUIRE(result.accepted);
    REQUIRE(result.filledSize == Quantity{0}); // rests fully, nothing crosses on its own side

    // Real book absorbs 4.0 of the 10.0 that was ahead of this order -- not
    // enough to reach it yet.
    book.bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("6.0"));
    engine.onBookDelta(BookSide::Bid, Price::fromDecimalString("100.0"), Quantity::fromDecimalString("6.0"));

    CHECK(recorder.fills.empty());
    CHECK(engine.cancelOrder(1).accepted); // still resting
}

TEST_CASE("a resting order fills once the book absorbs back past its own queued position", "[matching_engine]") {
    OrderBook book;
    book.bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("10.0"));

    FillRecorder recorder;
    MatchingEngine engine{"BTC/USD", book, recorder.callback()};

    auto result = engine.placeOrder(1, BookSide::Bid, OrderType::Limit, Price::fromDecimalString("100.0"),
                                     Quantity::fromDecimalString("5.0"));
    REQUIRE(result.accepted);
    REQUIRE(result.filledSize == Quantity{0});

    // New real bids join at the same price -- order 1's own queue-ahead
    // counter (10.0) is untouched by an increase.
    book.bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("15.0"));
    engine.onBookDelta(BookSide::Bid, Price::fromDecimalString("100.0"), Quantity::fromDecimalString("15.0"));
    CHECK(recorder.fills.empty());

    // The level clears out entirely -- a reduction of 15.0, which
    // overflows past order 1's frozen 10.0 by exactly 5.0, its full
    // remaining size.
    book.bids.applyDelta(Price::fromDecimalString("100.0"), Quantity{0});
    engine.onBookDelta(BookSide::Bid, Price::fromDecimalString("100.0"), Quantity{0});

    REQUIRE(recorder.fills.size() == 1);
    CHECK(recorder.fills[0].orderId == 1);
    CHECK(recorder.fills[0].price == Price::fromDecimalString("100.0"));
    CHECK(recorder.fills[0].size == Quantity::fromDecimalString("5.0"));
    CHECK_FALSE(engine.cancelOrder(1).accepted); // fully filled, erased
}

TEST_CASE("two orders resting at the same price fill in submission order", "[matching_engine]") {
    OrderBook book;
    book.bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("10.0"));

    FillRecorder recorder;
    MatchingEngine engine{"BTC/USD", book, recorder.callback()};

    // Order A joins first, when 10.0 is ahead of it.
    auto a = engine.placeOrder(1, BookSide::Bid, OrderType::Limit, Price::fromDecimalString("100.0"),
                                Quantity::fromDecimalString("3.0"));
    REQUIRE(a.accepted);
    REQUIRE(a.filledSize == Quantity{0});

    // More real volume joins before B arrives -- B's own queue-ahead
    // counter (15.0) starts larger than A's frozen 10.0.
    book.bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("15.0"));
    engine.onBookDelta(BookSide::Bid, Price::fromDecimalString("100.0"), Quantity::fromDecimalString("15.0"));

    auto b = engine.placeOrder(2, BookSide::Bid, OrderType::Limit, Price::fromDecimalString("100.0"),
                                Quantity::fromDecimalString("3.0"));
    REQUIRE(b.accepted);
    REQUIRE(b.filledSize == Quantity{0});

    // Reduction #1: overflows A's smaller position by 1.0, but doesn't
    // touch B's larger one at all.
    book.bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("4.0"));
    engine.onBookDelta(BookSide::Bid, Price::fromDecimalString("100.0"), Quantity::fromDecimalString("4.0"));

    REQUIRE(recorder.fills.size() == 1);
    CHECK(recorder.fills[0].orderId == 1);
    CHECK(recorder.fills[0].size == Quantity::fromDecimalString("1.0"));

    // Reduction #2: finishes off A's remaining 2.0; B still untouched.
    book.bids.applyDelta(Price::fromDecimalString("100.0"), Quantity{0});
    engine.onBookDelta(BookSide::Bid, Price::fromDecimalString("100.0"), Quantity{0});

    REQUIRE(recorder.fills.size() == 2);
    CHECK(recorder.fills[1].orderId == 1);
    CHECK(recorder.fills[1].size == Quantity::fromDecimalString("2.0"));
    CHECK_FALSE(engine.cancelOrder(1).accepted); // A fully filled and erased
    CHECK(engine.cancelOrder(2).accepted);       // B still resting -- never received a fill throughout

    for (const auto& f : recorder.fills) {
        CHECK(f.orderId == 1); // B never appears
    }
}

TEST_CASE("property: synchronous fill from placeOrder never exceeds the requested size", "[matching_engine]") {
    std::mt19937 rng{777};
    std::uniform_int_distribution<int> levelCountDist{0, 6};
    std::uniform_int_distribution<int> levelQtyDist{0, 4}; // whole units
    std::uniform_int_distribution<int> orderSizeDist{1, 20};
    std::uniform_int_distribution<int> orderTypeDist{0, 1};
    std::uniform_int_distribution<int> sideDist{0, 1};

    for (int trial = 0; trial < 2000; ++trial) {
        OrderBook book;
        BookSide side = sideDist(rng) == 0 ? BookSide::Bid : BookSide::Ask;
        OrderBookSide& opposite = side == BookSide::Bid ? book.asks : book.bids;

        int levelCount = levelCountDist(rng);
        for (int i = 0; i < levelCount; ++i) {
            Price p{static_cast<int64_t>(100 + i) * PRICE_SCALE};
            Quantity q{static_cast<int64_t>(levelQtyDist(rng)) * QUANTITY_SCALE};
            opposite.applyDelta(p, q);
        }

        FillRecorder recorder;
        MatchingEngine engine{"BTC/USD", book, recorder.callback()};

        OrderType type = orderTypeDist(rng) == 0 ? OrderType::Market : OrderType::Limit;
        Price limitPrice = Price::fromDecimalString("103.0"); // crosses roughly half the seeded levels
        Quantity size{static_cast<int64_t>(orderSizeDist(rng)) * QUANTITY_SCALE};

        auto result = engine.placeOrder(static_cast<uint64_t>(trial) + 1, side, type, limitPrice, size);
        REQUIRE(result.accepted);

        int64_t summedFills = 0;
        for (const auto& f : recorder.fills) {
            summedFills += f.size.ticks;
        }

        CHECK(summedFills == result.filledSize.ticks);
        CHECK(result.filledSize.ticks <= size.ticks);
    }
}
