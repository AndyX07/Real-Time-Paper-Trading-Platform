#include <catch2/catch_test_macros.hpp>

#include <random>
#include <vector>

#include "engine/book/checksum.hpp"
#include "engine/book/order_book.hpp"

TEST_CASE("applyDelta inserts new price levels in sorted order", "[order_book]") {
    OrderBookSide bids{BookSide::Bid};
    bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("1.0"));
    bids.applyDelta(Price::fromDecimalString("102.0"), Quantity::fromDecimalString("1.0"));
    bids.applyDelta(Price::fromDecimalString("101.0"), Quantity::fromDecimalString("1.0"));

    auto levels = bids.depth(3);
    REQUIRE(levels.size() == 3);
    // Bids sort best (highest) first.
    CHECK(levels[0].price == Price::fromDecimalString("102.0"));
    CHECK(levels[1].price == Price::fromDecimalString("101.0"));
    CHECK(levels[2].price == Price::fromDecimalString("100.0"));

    OrderBookSide asks{BookSide::Ask};
    asks.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("1.0"));
    asks.applyDelta(Price::fromDecimalString("102.0"), Quantity::fromDecimalString("1.0"));
    asks.applyDelta(Price::fromDecimalString("101.0"), Quantity::fromDecimalString("1.0"));

    auto askLevels = asks.depth(3);
    REQUIRE(askLevels.size() == 3);
    // Asks sort best (lowest) first.
    CHECK(askLevels[0].price == Price::fromDecimalString("100.0"));
    CHECK(askLevels[1].price == Price::fromDecimalString("101.0"));
    CHECK(askLevels[2].price == Price::fromDecimalString("102.0"));
}

TEST_CASE("applyDelta updates quantity at an existing price without reordering", "[order_book]") {
    OrderBookSide bids{BookSide::Bid};
    bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("1.0"));
    bids.applyDelta(Price::fromDecimalString("99.0"), Quantity::fromDecimalString("1.0"));

    bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("5.0"));

    CHECK(bids.depth(10).size() == 2);
    CHECK(bids.sizeAtPrice(Price::fromDecimalString("100.0")) == Quantity::fromDecimalString("5.0"));
}

TEST_CASE("applyDelta with zero quantity removes the level", "[order_book]") {
    OrderBookSide bids{BookSide::Bid};
    bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("1.0"));
    bids.applyDelta(Price::fromDecimalString("99.0"), Quantity::fromDecimalString("1.0"));

    bids.applyDelta(Price::fromDecimalString("100.0"), Quantity{0});

    CHECK(bids.depth(10).size() == 1);
    CHECK(bids.sizeAtPrice(Price::fromDecimalString("100.0")) == Quantity{0});
    CHECK(bids.sizeAtPrice(Price::fromDecimalString("99.0")) == Quantity::fromDecimalString("1.0"));
}

TEST_CASE("applyDelta removing a nonexistent price is a no-op", "[order_book]") {
    OrderBookSide bids{BookSide::Bid};
    bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("1.0"));

    auto evicted = bids.applyDelta(Price::fromDecimalString("50.0"), Quantity{0});

    CHECK_FALSE(evicted.has_value());
    CHECK(bids.depth(10).size() == 1);
}

TEST_CASE("applyDelta evicts the worst level once BOOK_DEPTH is exceeded", "[order_book]") {
    OrderBookSide bids{BookSide::Bid};
    for (size_t i = 0; i < BOOK_DEPTH; ++i) {
        // Highest price first so each subsequent insert is the new worst level.
        bids.applyDelta(Price{static_cast<int64_t>((BOOK_DEPTH - i) * PRICE_SCALE)}, Quantity::fromDecimalString("1.0"));
    }
    REQUIRE(bids.depth(BOOK_DEPTH + 1).size() == BOOK_DEPTH);

    Price worstBeforeInsert = bids.depth(BOOK_DEPTH).back().price;
    // Insert a new best-side price -- pushes the previous worst level out.
    auto evicted = bids.applyDelta(Price{static_cast<int64_t>((BOOK_DEPTH + 1) * PRICE_SCALE)},
                                    Quantity::fromDecimalString("1.0"));

    REQUIRE(evicted.has_value());
    CHECK(*evicted == worstBeforeInsert);
    CHECK(bids.depth(BOOK_DEPTH + 1).size() == BOOK_DEPTH);
}

TEST_CASE("top throws on an empty side", "[order_book]") {
    OrderBookSide bids{BookSide::Bid};
    CHECK_THROWS_AS(bids.top(), std::out_of_range);
}

TEST_CASE("sizeAtPrice returns zero for a price with no level", "[order_book]") {
    OrderBookSide bids{BookSide::Bid};
    bids.applyDelta(Price::fromDecimalString("100.0"), Quantity::fromDecimalString("1.0"));
    CHECK(bids.sizeAtPrice(Price::fromDecimalString("50.0")) == Quantity{0});
}

TEST_CASE("property: bid and ask sides never cross after a random sequence of deltas", "[order_book]") {
    std::mt19937 rng{12345};
    std::uniform_int_distribution<int> bidPriceDist{90, 110};
    std::uniform_int_distribution<int> askPriceDist{200, 220};
    std::uniform_int_distribution<int> qtyDist{0, 5}; // 0 exercises removal

    OrderBookSide bids{BookSide::Bid};
    OrderBookSide asks{BookSide::Ask};

    for (int step = 0; step < 5000; ++step) {
        Price bidPrice{static_cast<int64_t>(bidPriceDist(rng)) * PRICE_SCALE};
        Price askPrice{static_cast<int64_t>(askPriceDist(rng)) * PRICE_SCALE};
        Quantity qty{static_cast<int64_t>(qtyDist(rng)) * QUANTITY_SCALE};

        bids.applyDelta(bidPrice, qty);
        asks.applyDelta(askPrice, qty);

        if (!bids.depth(1).empty() && !asks.depth(1).empty()) {
            REQUIRE(bids.top().price < asks.top().price);
        }
    }
}

TEST_CASE("computeBookChecksum matches Kraken's published worked example", "[order_book][checksum]") {
    std::vector<PriceLevel> asks = {
        {Price::fromDecimalString("45285.2"), Quantity::fromDecimalString("0.00100000")},
        {Price::fromDecimalString("45286.4"), Quantity::fromDecimalString("1.54571953")},
        {Price::fromDecimalString("45286.6"), Quantity::fromDecimalString("1.54571109")},
        {Price::fromDecimalString("45289.6"), Quantity::fromDecimalString("1.54560911")},
        {Price::fromDecimalString("45290.2"), Quantity::fromDecimalString("0.15890660")},
        {Price::fromDecimalString("45291.8"), Quantity::fromDecimalString("1.54553491")},
        {Price::fromDecimalString("45294.7"), Quantity::fromDecimalString("0.04454749")},
        {Price::fromDecimalString("45296.1"), Quantity::fromDecimalString("0.35380000")},
        {Price::fromDecimalString("45297.5"), Quantity::fromDecimalString("0.09945542")},
        {Price::fromDecimalString("45299.5"), Quantity::fromDecimalString("0.18772827")},
    };
    std::vector<PriceLevel> bids = {
        {Price::fromDecimalString("45283.5"), Quantity::fromDecimalString("0.10000000")},
        {Price::fromDecimalString("45283.4"), Quantity::fromDecimalString("1.54582015")},
        {Price::fromDecimalString("45282.1"), Quantity::fromDecimalString("0.10000000")},
        {Price::fromDecimalString("45281.0"), Quantity::fromDecimalString("0.10000000")},
        {Price::fromDecimalString("45280.3"), Quantity::fromDecimalString("1.54592586")},
        {Price::fromDecimalString("45279.0"), Quantity::fromDecimalString("0.07990000")},
        {Price::fromDecimalString("45277.6"), Quantity::fromDecimalString("0.03310103")},
        {Price::fromDecimalString("45277.5"), Quantity::fromDecimalString("0.30000000")},
        {Price::fromDecimalString("45277.3"), Quantity::fromDecimalString("1.54602737")},
        {Price::fromDecimalString("45276.6"), Quantity::fromDecimalString("0.15445238")},
    };

    uint32_t result = computeBookChecksum(asks, bids, 1, 8);
    CHECK(result == 3310070434u);
}

TEST_CASE("computeBookChecksum only ever uses the top CHECKSUM_DEPTH levels", "[order_book][checksum]") {
    std::vector<PriceLevel> asks = {
        {Price::fromDecimalString("45285.2"), Quantity::fromDecimalString("0.00100000")},
        {Price::fromDecimalString("45286.4"), Quantity::fromDecimalString("1.54571953")},
        {Price::fromDecimalString("45286.6"), Quantity::fromDecimalString("1.54571109")},
        {Price::fromDecimalString("45289.6"), Quantity::fromDecimalString("1.54560911")},
        {Price::fromDecimalString("45290.2"), Quantity::fromDecimalString("0.15890660")},
        {Price::fromDecimalString("45291.8"), Quantity::fromDecimalString("1.54553491")},
        {Price::fromDecimalString("45294.7"), Quantity::fromDecimalString("0.04454749")},
        {Price::fromDecimalString("45296.1"), Quantity::fromDecimalString("0.35380000")},
        {Price::fromDecimalString("45297.5"), Quantity::fromDecimalString("0.09945542")},
        {Price::fromDecimalString("45299.5"), Quantity::fromDecimalString("0.18772827")},
    };
    std::vector<PriceLevel> bids = {
        {Price::fromDecimalString("45283.5"), Quantity::fromDecimalString("0.10000000")},
        {Price::fromDecimalString("45283.4"), Quantity::fromDecimalString("1.54582015")},
        {Price::fromDecimalString("45282.1"), Quantity::fromDecimalString("0.10000000")},
        {Price::fromDecimalString("45281.0"), Quantity::fromDecimalString("0.10000000")},
        {Price::fromDecimalString("45280.3"), Quantity::fromDecimalString("1.54592586")},
        {Price::fromDecimalString("45279.0"), Quantity::fromDecimalString("0.07990000")},
        {Price::fromDecimalString("45277.6"), Quantity::fromDecimalString("0.03310103")},
        {Price::fromDecimalString("45277.5"), Quantity::fromDecimalString("0.30000000")},
        {Price::fromDecimalString("45277.3"), Quantity::fromDecimalString("1.54602737")},
        {Price::fromDecimalString("45276.6"), Quantity::fromDecimalString("0.15445238")},
    };
    // An 11th ask level appended beyond CHECKSUM_DEPTH must not change the result.
    asks.push_back({Price::fromDecimalString("45999.9"), Quantity::fromDecimalString("9.99999999")});

    uint32_t result = computeBookChecksum(asks, bids, 1, 8);
    CHECK(result == 3310070434u);
}

TEST_CASE("stripForChecksum strips the decimal point and leading zeros", "[order_book][checksum]") {
    CHECK(checksum_detail::stripForChecksum("00452.85200000") == "45285200000");
    CHECK(checksum_detail::stripForChecksum("0.00100000") == "100000");
    CHECK(checksum_detail::stripForChecksum("0.00000000") == "0");
}
