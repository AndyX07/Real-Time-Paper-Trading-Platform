#pragma once

#include <atomic>
#include <condition_variable>
#include <functional>
#include <mutex>
#include <optional>
#include <string>

#include <boost/asio/io_context.hpp>
#include <boost/asio/ssl/context.hpp>
#include <boost/beast/core.hpp>
#include <boost/beast/ssl.hpp>
#include <boost/beast/websocket.hpp>
#include <boost/beast/websocket/ssl.hpp>

#include "engine/book/order_book.hpp"
#include "engine/market_data/kraken_parser.hpp"

namespace asio = boost::asio;
namespace beast = boost::beast;
namespace websocket = beast::websocket;
using tcp = asio::ip::tcp;

class KrakenBookClient {
public:

    using BookDeltaCallback = std::function<void(BookSide side, Price price, Quantity quantity, uint64_t seq)>;

    KrakenBookClient(std::string symbol, OrderBook& book, BookDeltaCallback onDelta);

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

    std::string symbol_;
    OrderBook& book_;
    BookDeltaCallback onDelta_;

    asio::io_context ioContext_;
    asio::ssl::context sslContext_{asio::ssl::context::tlsv13_client};
    std::optional<websocket::stream<beast::ssl_stream<beast::tcp_stream>>> ws_;
    simdjson::ondemand::parser parser_;

    std::mutex wsMutex_;
    std::atomic<bool> stopRequested_{false};

    std::mutex backoffMutex_;
    std::condition_variable backoffCv_;
};
