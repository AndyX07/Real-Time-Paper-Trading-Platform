#include "engine/market_data/kraken_book_client.hpp"

#include <algorithm>
#include <chrono>
#include <iostream>
#include <limits>
#include <stdexcept>

#include <boost/asio/ip/tcp.hpp>
#include <boost/beast/core/buffers_to_string.hpp>
#include <boost/system/error_code.hpp>

#include <openssl/ssl.h>

#include "engine/book/checksum.hpp"
#include "engine/market_data/idle_timeout_watchdog.hpp"
#include "engine/market_data/sync_timeout_guard.hpp"
#include "engine/observability/histogram.hpp"

namespace {

constexpr const char* kHost = "ws.kraken.com";
constexpr const char* kPort = "443";
constexpr const char* kTarget = "/v2";

constexpr std::chrono::milliseconds kInitialBackoff{500};
constexpr std::chrono::milliseconds kMaxBackoff{32000};
constexpr std::chrono::seconds kConnectTimeout{10};
constexpr std::chrono::seconds kReadIdleTimeout{60};

// threshold to publish book snapshot
constexpr uint64_t kSnapshotDeltaThreshold = 500;
constexpr std::chrono::seconds kSnapshotInterval{5};

}

KrakenBookClient::KrakenBookClient(std::string symbol, OrderBook& book, BookDeltaCallback onDelta,
                                    BookSnapshotCallback onSnapshot, int priceDecimals, int quantityDecimals,
                                    TickCallback onTick)
    : symbol_{std::move(symbol)}, book_{book}, onDelta_{std::move(onDelta)}, onSnapshot_{std::move(onSnapshot)},
      onTick_{std::move(onTick)}, priceDecimals_{priceDecimals}, quantityDecimals_{quantityDecimals} {}

void KrakenBookClient::connect() {
    namespace ssl = asio::ssl;

    // resolves dns
    tcp::resolver resolver{ioContext_};
    auto const results = resolver.resolve(kHost, kPort);

    {
        // guards against stop() accessing ws_
        std::lock_guard<std::mutex> lock{wsMutex_};
        ws_.emplace(ioContext_, sslContext_);
    }


    SyncTimeoutGuard timeoutGuard{beast::get_lowest_layer(*ws_), kConnectTimeout};

    // establish tcp connection
    beast::get_lowest_layer(*ws_).connect(results);

    // performs tls and ws handshakes
    if (!SSL_set_tlsext_host_name(ws_->next_layer().native_handle(), kHost)) {
        throw std::runtime_error("KrakenBookClient: failed to set TLS SNI hostname");
    }

    ws_->next_layer().handshake(ssl::stream_base::client);
    ws_->handshake(kHost, kTarget);
}

void KrakenBookClient::subscribe() {
    // No-op without a live socket -- true when handleMessage is driven
    // directly (tests, replay tooling) rather than through run()/connect().
    if (!ws_) {
        return;
    }
    std::string message = R"({"method":"subscribe","params":{"channel":"book","symbol":[")" +
                           symbol_ + R"("],"depth":)" + std::to_string(BOOK_DEPTH) + "}}";
    ws_->write(asio::buffer(message));
}

void KrakenBookClient::resubscribeAndRebuild() {
    if (ws_) {
        std::string unsubscribeMessage =
            R"({"method":"unsubscribe","params":{"channel":"book","symbol":[")" + symbol_ + R"("]}})";
        ws_->write(asio::buffer(unsubscribeMessage));
    }

    book_.bids.clear();
    book_.asks.clear();

    subscribe();
}

void KrakenBookClient::applyMessage(const BookMessage& message) {
    if (message.symbol != symbol_) {
        return;
    }

    if (message.type == BookMessageType::Snapshot) {
        book_.bids.clear();
        book_.asks.clear();
    }

    for (const auto& level : message.bids) {
        auto evicted = book_.bids.applyDelta(level.price, level.quantity);
        ++book_.lastSeq;
        ++deltasSinceSnapshot_;
        if (onDelta_) {
            onDelta_(BookSide::Bid, level.price, level.quantity, book_.lastSeq);
        }
        if (evicted) {
            ++book_.lastSeq;
            ++deltasSinceSnapshot_;
            if (onDelta_) {
                onDelta_(BookSide::Bid, *evicted, Quantity{0}, book_.lastSeq);
            }
        }
    }

    for (const auto& level : message.asks) {
        auto evicted = book_.asks.applyDelta(level.price, level.quantity);
        ++book_.lastSeq;
        ++deltasSinceSnapshot_;
        if (onDelta_) {
            onDelta_(BookSide::Ask, level.price, level.quantity, book_.lastSeq);
        }
        if (evicted) {
            ++book_.lastSeq;
            ++deltasSinceSnapshot_;
            if (onDelta_) {
                onDelta_(BookSide::Ask, *evicted, Quantity{0}, book_.lastSeq);
            }
        }
    }

    uint32_t computed = computeBookChecksum(book_.asks.depth(CHECKSUM_DEPTH), book_.bids.depth(CHECKSUM_DEPTH),
                                             priceDecimals_, quantityDecimals_);
    book_.lastCheckSum = computed;

    if (computed != message.checksum) {
        std::cerr << "engine: kraken_book_client[" << symbol_ << "] checksum mismatch on "
                  << (message.type == BookMessageType::Snapshot ? "snapshot" : "update")
                  << " computed=" << computed << " expected=" << message.checksum
                  << " bids=" << book_.bids.depth(CHECKSUM_DEPTH).size()
                  << " asks=" << book_.asks.depth(CHECKSUM_DEPTH).size()
                  << " msgBids=" << message.bids.size() << " msgAsks=" << message.asks.size() << "\n";
        resubscribeAndRebuild();
        return;
    }

    if (message.type == BookMessageType::Snapshot) {
        publishSnapshot();
    } else {
        maybePublishSnapshot();
    }
}

void KrakenBookClient::publishSnapshot() {
    if (onSnapshot_) {
        constexpr size_t kAll = std::numeric_limits<size_t>::max();
        onSnapshot_(book_.bids.depth(kAll), book_.asks.depth(kAll), book_.lastSeq);
    }
    deltasSinceSnapshot_ = 0;
    lastSnapshotTime_ = std::chrono::steady_clock::now();
}

void KrakenBookClient::maybePublishSnapshot() {
    bool dueByCount = deltasSinceSnapshot_ >= kSnapshotDeltaThreshold;
    bool dueByTime = (std::chrono::steady_clock::now() - lastSnapshotTime_) >= kSnapshotInterval;
    if (dueByCount || dueByTime) {
        publishSnapshot();
    }
}

void KrakenBookClient::handleMessage(std::string_view raw) {
    {
        ScopedLatencyTimer timer{HistogramRegistry::instance().get("book.tick_to_apply")};
        auto message = parseBookMessage(parser_, raw);
        if (message) {
            applyMessage(*message);
        }
    }

    if (onTick_) {
        onTick_();
    }
}

void KrakenBookClient::run() {
    auto backoff = kInitialBackoff;

    while (!stopRequested_.load(std::memory_order_relaxed)) {
        try {
            connect();
            subscribe();
            backoff = kInitialBackoff;

            beast::flat_buffer buffer;
            // kraken ws sends a heartbeat every second so if no response for 60 seconds, then dead connection
            IdleTimeoutWatchdog readWatchdog{beast::get_lowest_layer(*ws_), kReadIdleTimeout};
            while (!stopRequested_.load(std::memory_order_relaxed)) {
                buffer.clear();
                ws_->read(buffer);
                readWatchdog.kick();
                handleMessage(beast::buffers_to_string(buffer.data()));
            }
        } catch (const std::exception& e) {
            // Disconnect, handshake failure, or a malformed message
            // (kraken_parser.hpp) -- all handled the same way: fall
            // through to the backoff below and reconnect from scratch.
            // subscribe() on the new connection naturally requests a
            // fresh snapshot, so there's nothing else to clean up here.
            std::cerr << "engine: kraken_book_client[" << symbol_ << "] " << e.what() << "\n";
        }

        if (stopRequested_.load(std::memory_order_relaxed)) {
            break;
        }

        {
            std::unique_lock<std::mutex> lock{backoffMutex_};
            backoffCv_.wait_for(lock, backoff,
                                 [this] { return stopRequested_.load(std::memory_order_relaxed); });
        }
        backoff = std::min(backoff * 2, kMaxBackoff);
    }
}

void KrakenBookClient::stop() {
    stopRequested_.store(true, std::memory_order_relaxed);
    backoffCv_.notify_one();

    std::lock_guard<std::mutex> lock{wsMutex_};
    if (ws_) {
        auto& socket = beast::get_lowest_layer(*ws_).socket();
        boost::system::error_code ec;
        socket.close(ec);
    }
}
