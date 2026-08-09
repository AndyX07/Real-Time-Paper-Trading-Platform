#include <algorithm>
#include <atomic>
#include <chrono>
#include <cstdio>
#include <cstdlib>
#include <fstream>
#include <iostream>
#include <sstream>
#include <string>
#include <thread>

#include "engine/ipc/ring_buffer.hpp"
#include "engine/market_data/kraken_book_client.hpp"
#include "engine/observability/histogram.hpp"
#include "engine/replay/recorded_session.hpp"

namespace {

struct Args {
    std::string mode;
    std::string inPath;
    std::string speed = "real";
    int factor = 1;
    bool measurePop = false;
    std::string goldenPath;
    bool updateGolden = false;
    int priceDecimals = 1;
    int quantityDecimals = 8;
    std::string symbol = "BTC/USD";
};

Args parseArgs(int argc, char** argv) {
    if (argc < 2) {
        std::fprintf(stderr, "usage: kraken_replayer <bench|verify> --in <path> [options]\n");
        std::exit(2);
    }
    Args args;
    args.mode = argv[1];
    for (int i = 2; i < argc; ++i) {
        std::string arg = argv[i];
        auto next = [&] { return std::string(argv[++i]); };
        if (arg == "--in") {
            args.inPath = next();
        } else if (arg == "--speed") {
            args.speed = next();
        } else if (arg == "--factor") {
            args.factor = std::atoi(next().c_str());
        } else if (arg == "--measure-pop") {
            args.measurePop = true;
        } else if (arg == "--golden") {
            args.goldenPath = next();
        } else if (arg == "--update-golden") {
            args.updateGolden = true;
        } else if (arg == "--symbol") {
            args.symbol = next();
        }
    }
    if (args.inPath.empty()) {
        std::fprintf(stderr, "kraken_replayer: --in is required\n");
        std::exit(2);
    }
    return args;
}

std::vector<RecordedFrame> loadFrames(const std::string& path) {
    std::ifstream in{path, std::ios::binary};
    if (!in) {
        std::fprintf(stderr, "kraken_replayer: failed to open %s\n", path.c_str());
        std::exit(1);
    }
    return readAllRecordedFrames(in);
}

int runBench(const Args& args) {
    std::vector<RecordedFrame> frames = loadFrames(args.inPath);
    std::cout << "kraken_replayer: loaded " << frames.size() << " frames from " << args.inPath << "\n";

    OrderBook book;
    BookDeltaRingBuffer ring;

    KrakenBookClient::BookDeltaCallback onDelta = [&](BookSide side, Price price, Quantity qty, uint64_t seq) {
        BookDelta delta{};
        setSymbol(delta, args.symbol);
        delta.seq = seq;
        delta.side = toWireSide(side);
        delta.price = price;
        delta.size = qty;
        ScopedLatencyTimer timer{HistogramRegistry::instance().get("ipc.ring_push")};
        ring.tryPush(delta);
    };
    KrakenBookClient client{args.symbol,
                            book,
                            onDelta,
                            [](std::span<const PriceLevel>, std::span<const PriceLevel>, uint64_t) {},
                            args.priceDecimals,
                            args.quantityDecimals,
                            [] {}};

    std::atomic<bool> stopPopping{false};
    std::thread popThread;
    if (args.measurePop) {
        popThread = std::thread{[&] {
            BookDelta out;
            while (!stopPopping.load(std::memory_order_relaxed)) {
                bool popped;
                {
                    // Times only the tryPop call itself -- the idle-wait
                    // sleep below is this thread polling for more data,
                    // not part of the pop's own cost.
                    ScopedLatencyTimer timer{HistogramRegistry::instance().get("ipc.ring_pop")};
                    popped = ring.tryPop(out);
                }
                if (!popped) {
                    std::this_thread::sleep_for(std::chrono::microseconds{10});
                }
            }
        }};
    }

    auto start = std::chrono::steady_clock::now();
    for (size_t i = 0; i < frames.size(); ++i) {
        if (args.speed != "burst" && i > 0) {
            uint64_t gapNanos = frames[i].recvTsNanos - frames[i - 1].recvTsNanos;
            uint64_t sleepNanos = args.speed == "Nx" ? gapNanos / static_cast<uint64_t>(std::max(1, args.factor))
                                                       : gapNanos;
            if (sleepNanos > 0) {
                std::this_thread::sleep_for(std::chrono::nanoseconds{sleepNanos});
            }
        }
        client.handleMessage(frames[i].raw);
    }
    auto elapsed = std::chrono::steady_clock::now() - start;

    stopPopping.store(true);
    if (popThread.joinable()) {
        popThread.join();
    }

    std::cout << "kraken_replayer: replayed " << frames.size() << " frames in "
              << std::chrono::duration_cast<std::chrono::milliseconds>(elapsed).count() << "ms\n";
    for (const auto& s : HistogramRegistry::instance().snapshotAll()) {
        if (s.count == 0) continue;
        std::cout << "  " << s.name << " count=" << s.count << " p50=" << s.p50 << "ns p90=" << s.p90
                  << "ns p99=" << s.p99 << "ns max=" << s.max << "ns\n";
    }
    return 0;
}

int runVerify(const Args& args) {
    std::vector<RecordedFrame> frames = loadFrames(args.inPath);

    OrderBook book;
    KrakenBookClient client{args.symbol,
                            book,
                            [](BookSide, Price, Quantity, uint64_t) {},
                            [](std::span<const PriceLevel>, std::span<const PriceLevel>, uint64_t) {},
                            args.priceDecimals,
                            args.quantityDecimals,
                            [] {}};

    std::ostringstream actual;
    for (const auto& frame : frames) {
        client.handleMessage(frame.raw);
        actual << book.lastSeq << " " << book.lastCheckSum << "\n";
    }

    if (args.updateGolden) {
        std::ofstream out{args.goldenPath, std::ios::trunc};
        out << actual.str();
        std::cout << "kraken_replayer: wrote golden file " << args.goldenPath << "\n";
        return 0;
    }

    std::ifstream goldenIn{args.goldenPath};
    if (!goldenIn) {
        std::fprintf(stderr, "kraken_replayer: failed to open golden file %s\n", args.goldenPath.c_str());
        return 1;
    }
    std::ostringstream goldenBuf;
    goldenBuf << goldenIn.rdbuf();

    if (actual.str() != goldenBuf.str()) {
        std::fprintf(stderr, "kraken_replayer: MISMATCH against golden file %s\n", args.goldenPath.c_str());
        return 1;
    }
    std::cout << "kraken_replayer: matches golden file (" << frames.size() << " frames)\n";
    return 0;
}

}

int main(int argc, char** argv) {
    Args args = parseArgs(argc, argv);
    if (args.mode == "bench") {
        return runBench(args);
    }
    if (args.mode == "verify") {
        if (args.goldenPath.empty()) {
            std::fprintf(stderr, "kraken_replayer: --golden is required for verify\n");
            return 2;
        }
        return runVerify(args);
    }
    std::fprintf(stderr, "kraken_replayer: unknown mode %s\n", args.mode.c_str());
    return 2;
}
