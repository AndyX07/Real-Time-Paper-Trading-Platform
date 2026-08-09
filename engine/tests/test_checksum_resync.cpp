#include <catch2/catch_test_macros.hpp>

#include <algorithm>
#include <fstream>
#include <sstream>
#include <vector>

#include "engine/book/checksum.hpp"
#include "engine/market_data/kraken_book_client.hpp"
#include "engine/replay/recorded_session.hpp"

namespace {

std::string levelJson(const PriceLevel& level) {
    // Kraken sends price/qty as raw JSON numbers, not strings --
    // raw_json_token() includes the surrounding quotes for a JSON string
    // token, which would otherwise overflow parseFixedPoint's
    // fractional-digit limit once combined with toDecimalString()'s full
    // 10-digit padding.
    std::ostringstream out;
    out << R"({"price":)" << level.price.toDecimalString() << R"(,"qty":)" << level.quantity.toDecimalString() << "}";
    return out.str();
}

std::string levelsJson(const std::vector<PriceLevel>& levels) {
    std::ostringstream out;
    out << "[";
    for (size_t i = 0; i < levels.size(); ++i) {
        if (i > 0) out << ",";
        out << levelJson(levels[i]);
    }
    out << "]";
    return out.str();
}

std::string bookMessageJson(std::string_view symbol, std::string_view type, const std::vector<PriceLevel>& bids,
                             const std::vector<PriceLevel>& asks, uint32_t checksum) {
    std::ostringstream out;
    out << R"({"channel":"book","type":")" << type << R"(","data":[{"symbol":")" << symbol << R"(","bids":)"
        << levelsJson(bids) << R"(,"asks":)" << levelsJson(asks) << R"(,"checksum":)" << checksum << "}]}";
    return out.str();
}

// Small fixed test book -- 2 levels/side, native precision matching the
// decimal strings used (1 price decimal, 2 quantity decimals).
std::vector<PriceLevel> testBids() {
    return {
        {Price::fromDecimalString("100.1"), Quantity::fromDecimalString("1.50")},
        {Price::fromDecimalString("99.5"), Quantity::fromDecimalString("2.00")},
    };
}

std::vector<PriceLevel> testAsks() {
    return {
        {Price::fromDecimalString("101.2"), Quantity::fromDecimalString("1.00")},
        {Price::fromDecimalString("102.0"), Quantity::fromDecimalString("0.75")},
    };
}

uint32_t testChecksum() {
    return computeBookChecksum(testAsks(), testBids(), 1, 2);
}

// Records callback activity without needing a live socket.
struct Harness {
    int deltaCount = 0;
    int snapshotCount = 0;
    OrderBook book;
    KrakenBookClient client;

    explicit Harness(int priceDecimals = 1, int quantityDecimals = 2)
        : client{
              "BTC/USD",
              book,
              [this](BookSide, Price, Quantity, uint64_t) { ++deltaCount; },
              [this](std::span<const PriceLevel>, std::span<const PriceLevel>, uint64_t) { ++snapshotCount; },
              priceDecimals,
              quantityDecimals,
              [] {},
          } {}
};

// Kraken's real checksum field is embedded as a bare JSON number
// ("checksum":123) -- swapping in a value that differs but has the same
// digit count keeps the frame's byte length (and therefore every other
// field's offset) unchanged, so this is a safe, surgical corruption of
// only the field this test cares about.
std::string corruptChecksum(std::string raw) {
    size_t pos = raw.find(R"("checksum":)");
    REQUIRE(pos != std::string::npos);
    size_t digitsStart = pos + std::string(R"("checksum":)").size();
    size_t digitsEnd = raw.find_first_not_of("0123456789", digitsStart);
    std::string original = raw.substr(digitsStart, digitsEnd - digitsStart);
    std::string corrupted(original.size(), '9');
    if (corrupted == original) {
        corrupted.assign(original.size(), '1');
    }
    raw.replace(digitsStart, original.size(), corrupted);
    return raw;
}

}

TEST_CASE("handleMessage applies a valid snapshot and computes a matching checksum", "[checksum_resync]") {
    Harness h;
    h.client.handleMessage(bookMessageJson("BTC/USD", "snapshot", testBids(), testAsks(), testChecksum()));

    CHECK(h.snapshotCount == 1);
    CHECK(h.book.bids.depth(10).size() == 2);
    CHECK(h.book.asks.depth(10).size() == 2);
    CHECK(h.book.lastCheckSum == testChecksum());
}

TEST_CASE("a corrupted update triggers resubscribeAndRebuild and clears book state", "[checksum_resync]") {
    Harness h;
    h.client.handleMessage(bookMessageJson("BTC/USD", "snapshot", testBids(), testAsks(), testChecksum()));
    REQUIRE(h.book.bids.depth(10).size() == 2);

    // A tiny update with a deliberately wrong checksum.
    std::vector<PriceLevel> tinyBidUpdate = {{Price::fromDecimalString("100.1"), Quantity::fromDecimalString("1.00")}};
    h.client.handleMessage(bookMessageJson("BTC/USD", "update", tinyBidUpdate, {}, 1));

    // resubscribeAndRebuild clears both sides -- proves the mismatch was
    // detected and handled, not silently accepted.
    CHECK(h.book.bids.depth(10).empty());
    CHECK(h.book.asks.depth(10).empty());
    // No further snapshot/delta callback fired for the corrupted message
    // itself -- applyMessage returns immediately after rebuilding.
    CHECK(h.snapshotCount == 1); // still just the one from the initial valid snapshot
}

TEST_CASE("the engine resumes correctly once a valid snapshot follows a corrupted diff", "[checksum_resync]") {
    Harness h;
    h.client.handleMessage(bookMessageJson("BTC/USD", "snapshot", testBids(), testAsks(), testChecksum()));

    std::vector<PriceLevel> tinyBidUpdate = {{Price::fromDecimalString("100.1"), Quantity::fromDecimalString("1.00")}};
    h.client.handleMessage(bookMessageJson("BTC/USD", "update", tinyBidUpdate, {}, 1));
    REQUIRE(h.book.bids.depth(10).empty());

    // Detect -> drop -> resnapshot -> resume, end to end.
    h.client.handleMessage(bookMessageJson("BTC/USD", "snapshot", testBids(), testAsks(), testChecksum()));

    CHECK(h.book.bids.depth(10).size() == 2);
    CHECK(h.book.asks.depth(10).size() == 2);
    CHECK(h.book.lastCheckSum == testChecksum());
    CHECK(h.snapshotCount == 2);
}

TEST_CASE("a message for a different symbol is ignored", "[checksum_resync]") {
    Harness h; // constructed for "BTC/USD"
    h.client.handleMessage(bookMessageJson("ETH/USD", "snapshot", testBids(), testAsks(), testChecksum()));

    CHECK(h.book.bids.depth(10).empty());
    CHECK(h.book.asks.depth(10).empty());
    CHECK(h.snapshotCount == 0);
}

TEST_CASE("a non-book message (e.g. heartbeat) is ignored without throwing", "[checksum_resync]") {
    Harness h;
    CHECK_NOTHROW(h.client.handleMessage(R"({"channel":"heartbeat"})"));
    CHECK_NOTHROW(h.client.handleMessage(R"({"method":"subscribe","result":{"channel":"book"}})"));

    CHECK(h.book.bids.depth(10).empty());
    CHECK(h.snapshotCount == 0);
}

TEST_CASE("a corrupted frame from a real recorded session triggers detect -> drop -> resnapshot -> resume",
          "[checksum_resync][replay]") {
    std::ifstream in{CHECKSUM_RESYNC_FIXTURE_PATH, std::ios::binary};
    REQUIRE(in.good());
    std::vector<RecordedFrame> frames = readAllRecordedFrames(in);

    // A live Kraken session's first frames aren't all book messages
    // (connection status, the subscribe ack, occasional heartbeats) --
    // find the actual first snapshot and the first update that follows
    // it, rather than assuming fixed indices.
    auto isBookType = [](const std::string& raw, const char* type) {
        return raw.find(R"("channel":"book")") != std::string::npos &&
               raw.find(std::string(R"("type":")") + type + "\"") != std::string::npos;
    };
    auto snapshotIt = std::find_if(frames.begin(), frames.end(),
                                    [&](const RecordedFrame& f) { return isBookType(f.raw, "snapshot"); });
    REQUIRE(snapshotIt != frames.end());
    auto updateIt = std::find_if(snapshotIt + 1, frames.end(),
                                  [&](const RecordedFrame& f) { return isBookType(f.raw, "update"); });
    REQUIRE(updateIt != frames.end());

    Harness h{1, 8};

    // The real snapshot applies cleanly against real Kraken data.
    h.client.handleMessage(snapshotIt->raw);
    REQUIRE(h.snapshotCount == 1);
    REQUIRE_FALSE(h.book.bids.depth(1).empty());
    uint32_t checksumBeforeCorruption = h.book.lastCheckSum;

    // Corrupt the next real update's checksum field and feed it -- must
    // be detected and trigger a full rebuild (both sides clear).
    h.client.handleMessage(corruptChecksum(updateIt->raw));
    CHECK(h.book.bids.depth(1).empty());
    CHECK(h.book.asks.depth(1).empty());

    // Resume: re-feed the original (uncorrupted) snapshot frame and
    // confirm the book repopulates and checksums correctly again.
    h.client.handleMessage(snapshotIt->raw);
    CHECK(h.snapshotCount == 2);
    CHECK_FALSE(h.book.bids.depth(1).empty());
    CHECK(h.book.lastCheckSum == checksumBeforeCorruption);
}
