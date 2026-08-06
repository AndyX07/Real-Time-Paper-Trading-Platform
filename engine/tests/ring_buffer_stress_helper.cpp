#include <chrono>
#include <cstdint>
#include <cstdio>
#include <fstream>
#include <string>
#include <thread>

#include <boost/interprocess/mapped_region.hpp>
#include <boost/interprocess/shared_memory_object.hpp>

#include "engine/ipc/ring_buffer.hpp"

namespace bip = boost::interprocess;

namespace {

std::string readyMarkerPath(const std::string& resultFile) {
    return resultFile + ".ready";
}

BookDelta makeDelta(uint64_t seq) {
    BookDelta delta{};
    setSymbol(delta, "TEST");
    delta.seq = seq;
    delta.engineTsNanos = seq;
    delta.side = 0;
    delta.price = Price{static_cast<int64_t>(seq) * 7};
    delta.size = Quantity{static_cast<int64_t>(seq) * 13};
    return delta;
}

bool deltaMatchesSeq(const BookDelta& d) {
    return d.price.ticks == static_cast<int64_t>(d.seq) * 7 && d.size.ticks == static_cast<int64_t>(d.seq) * 13;
}

int runWriter(const std::string& segmentName, uint64_t count, const std::string& resultFile) {
    std::remove(readyMarkerPath(resultFile).c_str());

    bip::shared_memory_object::remove(segmentName.c_str());
    bip::shared_memory_object segment{bip::create_only, segmentName.c_str(), bip::read_write};
    segment.truncate(static_cast<bip::offset_t>(sizeof(BookDeltaRingBuffer)));
    bip::mapped_region region{segment, bip::read_write};
    auto* ring = new (region.get_address()) BookDeltaRingBuffer();

    std::ofstream(readyMarkerPath(resultFile)) << "ready\n";

    for (uint64_t seq = 1; seq <= count; ++seq) {
        BookDelta delta = makeDelta(seq);
        auto giveUpAt = std::chrono::steady_clock::now() + std::chrono::seconds{30};
        while (!ring->tryPush(delta)) {
            if (std::chrono::steady_clock::now() > giveUpAt) {
                std::ofstream(resultFile) << "FAIL writer gave up retrying push at seq " << seq << " of " << count
                                            << "\n";
                return 1;
            }
            std::this_thread::sleep_for(std::chrono::microseconds{50});
        }
    }
    return 0;
}

int runReader(const std::string& segmentName, uint64_t count, const std::string& resultFile) {
    auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds{10};
    while (!std::ifstream(readyMarkerPath(resultFile)).good()) {
        if (std::chrono::steady_clock::now() > deadline) {
            std::ofstream(resultFile) << "FAIL writer never signaled ready\n";
            return 1;
        }
        std::this_thread::sleep_for(std::chrono::milliseconds{10});
    }

    bip::shared_memory_object segment{bip::open_only, segmentName.c_str(), bip::read_write};
    bip::mapped_region region{segment, bip::read_write};
    auto* ring = static_cast<BookDeltaRingBuffer*>(region.get_address());

    uint64_t received = 0;
    uint64_t expectedSeq = 1;
    deadline = std::chrono::steady_clock::now() + std::chrono::seconds{30};
    while (received < count) {
        BookDelta delta;
        if (!ring->tryPop(delta)) {
            if (std::chrono::steady_clock::now() > deadline) {
                std::ofstream(resultFile) << "FAIL timed out at " << received << " of " << count << "\n";
                return 1;
            }
            std::this_thread::sleep_for(std::chrono::microseconds{50});
            continue;
        }
        if (delta.seq != expectedSeq) {
            std::ofstream(resultFile) << "FAIL out-of-order or missing: expected seq " << expectedSeq << " got "
                                        << delta.seq << "\n";
            return 1;
        }
        if (!deltaMatchesSeq(delta)) {
            std::ofstream(resultFile) << "FAIL torn read at seq " << delta.seq << "\n";
            return 1;
        }
        ++received;
        ++expectedSeq;
    }
    std::ofstream(resultFile) << "OK " << received << "\n";
    return 0;
}

}

int main(int argc, char** argv) {
    if (argc != 5) {
        std::fprintf(stderr, "usage: %s <writer|reader> <segment-name> <count> <result-file>\n", argv[0]);
        return 2;
    }
    std::string role = argv[1];
    std::string segmentName = argv[2];
    uint64_t count = std::stoull(argv[3]);
    std::string resultFile = argv[4];

    if (role == "writer") {
        return runWriter(segmentName, count, resultFile);
    }
    if (role == "reader") {
        return runReader(segmentName, count, resultFile);
    }
    std::fprintf(stderr, "unknown role: %s\n", role.c_str());
    return 2;
}
