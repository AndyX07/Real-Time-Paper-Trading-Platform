#include <chrono>
#include <cstdio>
#include <cstdlib>
#include <fstream>
#include <iostream>
#include <string>
#include <thread>

#include <boost/asio/connect.hpp>
#include <boost/asio/io_context.hpp>
#include <boost/asio/ip/tcp.hpp>
#include <boost/asio/signal_set.hpp>
#include <boost/asio/ssl/context.hpp>
#include <boost/asio/ssl/stream.hpp>
#include <boost/beast/core.hpp>
#include <boost/beast/ssl.hpp>
#include <boost/beast/websocket.hpp>
#include <boost/beast/websocket/ssl.hpp>

#include <openssl/ssl.h>

#include "recorded_session.hpp"

namespace asio = boost::asio;
namespace beast = boost::beast;
namespace websocket = beast::websocket;
namespace ssl = asio::ssl;
using tcp = asio::ip::tcp;

namespace {

constexpr const char* kHost = "ws.kraken.com";
constexpr const char* kPort = "443";
constexpr const char* kTarget = "/v2";

uint64_t nowNanos() {
    return static_cast<uint64_t>(
        std::chrono::duration_cast<std::chrono::nanoseconds>(std::chrono::system_clock::now().time_since_epoch())
            .count());
}

struct Args {
    std::string symbol = "BTC/USD";
    std::string outPath;
    int durationSeconds = 300;
};

Args parseArgs(int argc, char** argv) {
    Args args;
    for (int i = 1; i < argc; ++i) {
        std::string arg = argv[i];
        if (arg == "--symbol" && i + 1 < argc) {
            args.symbol = argv[++i];
        } else if (arg == "--out" && i + 1 < argc) {
            args.outPath = argv[++i];
        } else if (arg == "--duration" && i + 1 < argc) {
            args.durationSeconds = std::atoi(argv[++i]);
        }
    }
    if (args.outPath.empty()) {
        std::fprintf(stderr, "usage: kraken_recorder --symbol <symbol> --out <path> [--duration <seconds>]\n");
        std::exit(2);
    }
    return args;
}

}

int main(int argc, char** argv) {
    Args args = parseArgs(argc, argv);

    std::ofstream out{args.outPath, std::ios::binary | std::ios::trunc};
    if (!out) {
        std::fprintf(stderr, "kraken_recorder: failed to open %s for writing\n", args.outPath.c_str());
        return 1;
    }
    writeRecordedSessionHeader(out);

    asio::io_context ioContext;
    ssl::context sslContext{ssl::context::tlsv13_client};
    tcp::resolver resolver{ioContext};
    websocket::stream<beast::ssl_stream<beast::tcp_stream>> ws{ioContext, sslContext};

    auto results = resolver.resolve(kHost, kPort);
    beast::get_lowest_layer(ws).connect(results);

    if (!SSL_set_tlsext_host_name(ws.next_layer().native_handle(), kHost)) {
        std::fprintf(stderr, "kraken_recorder: failed to set TLS SNI hostname\n");
        return 1;
    }
    ws.next_layer().handshake(ssl::stream_base::client);
    ws.handshake(kHost, kTarget);

    std::string subscribeMessage = R"({"method":"subscribe","params":{"channel":"book","symbol":[")" + args.symbol +
                                    R"("],"depth":100}})";
    ws.write(asio::buffer(subscribeMessage));

    // Ctrl-C stops the read loop below cleanly (closes the socket, which
    // makes ws.read() throw and fall out of the loop) in addition to the
    // --duration bound.
    bool stopRequested = false;
    asio::signal_set signals{ioContext, SIGINT, SIGTERM};
    signals.async_wait([&](const boost::system::error_code&, int) {
        stopRequested = true;
        beast::get_lowest_layer(ws).close();
    });
    std::thread signalThread{[&] { ioContext.run(); }};

    auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds{args.durationSeconds};
    uint64_t frameCount = 0;
    beast::flat_buffer buffer;

    std::cout << "kraken_recorder: recording " << args.symbol << " to " << args.outPath << " for "
              << args.durationSeconds << "s (Ctrl-C to stop early)\n";

    try {
        while (!stopRequested && std::chrono::steady_clock::now() < deadline) {
            buffer.clear();
            ws.read(buffer);
            std::string raw = beast::buffers_to_string(buffer.data());
            writeRecordedFrame(out, nowNanos(), raw);
            ++frameCount;
        }
    } catch (const std::exception& e) {
        if (!stopRequested) {
            std::fprintf(stderr, "kraken_recorder: connection error: %s\n", e.what());
        }
    }

    ioContext.stop();
    signalThread.join();
    out.flush();

    std::cout << "kraken_recorder: recorded " << frameCount << " frames\n";
    return 0;
}
