#pragma once

#include <atomic>
#include <chrono>
#include <condition_variable>
#include <functional>
#include <mutex>
#include <optional>
#include <span>
#include <string>

#include <boost/asio/io_context.hpp>
#include <boost/asio/ssl/context.hpp>
#include <boost/beast/core.hpp>
#include <boost/beast/ssl.hpp>
#include <boost/beast/websocket.hpp>
#include <boost/beast/websocket/ssl.hpp>

#include "engine/book/order_book.hpp"
#include "engine/book/price_level.hpp"
#include "engine/market_data/kraken_parser.hpp"

namespace asio = boost::asio;
namespace beast = boost::beast;
namespace websocket = beast::websocket;
using tcp = asio::ip::tcp;

class KrakenBookClient {
public:

    using BookDeltaCallback = std::function<void(BookSide side, Price price, Quantity quantity, uint64_t seq)>;

    using BookSnapshotCallback =
        std::function<void(std::span<const PriceLevel> bids, std::span<const PriceLevel> asks, uint64_t seq)>;

    using TickCallback = std::function<void()>;

    KrakenBookClient(std::string symbol, OrderBook& book, BookDeltaCallback onDelta,
                      BookSnapshotCallback onSnapshot, int priceDecimals, int quantityDecimals, TickCallback onTick);

    // not copyable or movable
    KrakenBookClient(const KrakenBookClient&) = delete;
    KrakenBookClient& operator=(const KrakenBookClient&) = delete;

    void run();
    void stop();

private:
    void connect();
    void subscribe();
    void resubscribeAndRebuild();
    void handleMessage(std::string_view raw);
    void applyMessage(const BookMessage& message);

    void publishSnapshot();
    void maybePublishSnapshot();

    std::string symbol_;
    OrderBook& book_;
    BookDeltaCallback onDelta_;
    BookSnapshotCallback onSnapshot_;
    TickCallback onTick_;
    int priceDecimals_;
    int quantityDecimals_;
    uint64_t deltasSinceSnapshot_ = 0;
    std::chrono::steady_clock::time_point lastSnapshotTime_ = std::chrono::steady_clock::now();

    asio::io_context ioContext_;
    asio::ssl::context sslContext_{asio::ssl::context::tlsv13_client};
    std::optional<websocket::stream<beast::ssl_stream<beast::tcp_stream>>> ws_;
    simdjson::ondemand::parser parser_;

    std::mutex wsMutex_;
    std::atomic<bool> stopRequested_{false};

    std::mutex backoffMutex_;
    std::condition_variable backoffCv_;
};
