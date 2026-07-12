#include "engine/market_data/kraken_asset_pairs.hpp"

#include <cctype>
#include <chrono>
#include <iomanip>
#include <sstream>
#include <string>

#include <boost/asio/ip/tcp.hpp>
#include <boost/asio/ssl.hpp>
#include <boost/beast/core.hpp>
#include <boost/beast/http.hpp>
#include <boost/beast/ssl.hpp>

#include <openssl/ssl.h>

#include <simdjson.h>

#include "engine/market_data/sync_timeout_guard.hpp"

namespace {

namespace asio = boost::asio;
namespace beast = boost::beast;
namespace http = beast::http;
using tcp = asio::ip::tcp;

constexpr const char* kHost = "api.kraken.com";
constexpr const char* kPort = "443";

constexpr std::chrono::seconds kRequestTimeout{10};

std::string urlEncode(std::string_view value) {
    std::ostringstream out;
    out << std::hex << std::uppercase;
    for (unsigned char c : value) {
        if (std::isalnum(c) || c == '-' || c == '_' || c == '.' || c == '~') {
            out << static_cast<char>(c);
        } else {
            out << '%' << std::setw(2) << std::setfill('0') << static_cast<int>(c);
        }
    }
    return out.str();
}

}

std::optional<SymbolPrecision> fetchKrakenAssetPairPrecision(std::string_view symbol) {
    try {
        asio::io_context ioContext;
        asio::ssl::context sslContext{asio::ssl::context::tlsv13_client};

        tcp::resolver resolver(ioContext);
        auto const results = resolver.resolve(kHost, kPort);

        beast::ssl_stream<beast::tcp_stream> stream(ioContext, sslContext);
        if (!SSL_set_tlsext_host_name(stream.native_handle(), kHost)) {
            return std::nullopt;
        }

        SyncTimeoutGuard timeoutGuard(beast::get_lowest_layer(stream), kRequestTimeout);

        beast::get_lowest_layer(stream).connect(results);
        stream.handshake(asio::ssl::stream_base::client);

        std::string target = "/0/public/AssetPairs?pair=" + urlEncode(symbol);
        http::request<http::string_body> req{http::verb::get, target, 11};
        req.set(http::field::host, kHost);
        req.set(http::field::user_agent, "paper-trader-engine");
        http::write(stream, req);

        beast::flat_buffer buffer;
        http::response<http::string_body> res;
        http::read(stream, buffer, res);

        boost::system::error_code ec;
        stream.shutdown(ec);

        if (res.result() != http::status::ok) {
            return std::nullopt;
        }

        simdjson::padded_string json(res.body());
        simdjson::ondemand::parser parser;
        simdjson::ondemand::document doc = parser.iterate(json);

        simdjson::ondemand::array errors = doc["error"].get_array();
        if (errors.count_elements().value() > 0) {
            return std::nullopt;
        }

        simdjson::ondemand::object result = doc["result"].get_object();
        for (auto field : result) {
            simdjson::ondemand::object pairInfo = field.value().get_object();
            int64_t pairDecimals = pairInfo["pair_decimals"].get_int64();
            int64_t lotDecimals = pairInfo["lot_decimals"].get_int64();
            return SymbolPrecision{static_cast<int>(pairDecimals), static_cast<int>(lotDecimals)};
        }
        return std::nullopt; // empty result object -- unrecognized pair
    } catch (const std::exception&) {
        return std::nullopt;
    }
}
