#include <catch2/catch_test_macros.hpp>

#include <chrono>
#include <cstdio>
#include <fstream>
#include <sstream>
#include <string>

#include <process.h>

#include "engine/ipc/ring_buffer.hpp"

namespace {

std::string tempFilePath(const std::string& name) {
    char* tmpDir = std::getenv("TEMP");
    return std::string(tmpDir ? tmpDir : ".") + "\\" + name;
}

// Spawns RING_BUFFER_STRESS_HELPER_PATH with the given args, does not
// wait for it -- returns the OS process handle. _spawnl reconstructs a
// single command-line string from its variadic args by concatenating
// them with spaces, so any argument containing a space (this repo's own
// path included -- "Paper Trader") must be pre-quoted, or the child's
// own argv-splitting will see it as multiple arguments.
std::string quoteArg(const std::string& s) {
    return "\"" + s + "\"";
}

intptr_t spawnHelper(const std::string& role, const std::string& segmentName, uint64_t count,
                     const std::string& resultFile) {
    std::string exePath = RING_BUFFER_STRESS_HELPER_PATH;
    std::string quotedExe = quoteArg(exePath);
    std::string quotedResult = quoteArg(resultFile);
    return _spawnl(_P_NOWAIT, exePath.c_str(), quotedExe.c_str(), role.c_str(), segmentName.c_str(),
                   std::to_string(count).c_str(), quotedResult.c_str(), static_cast<char*>(nullptr));
}

int waitForExit(intptr_t handle) {
    int termStatus = 0;
    _cwait(&termStatus, handle, _WAIT_CHILD);
    return termStatus;
}

}

TEST_CASE("cross-process: every pushed BookDelta is received exactly once, in order, untorn", "[ring_buffer][ipc]") {
    const std::string segmentName = "paper_trader_test_ring_stress";
    const std::string resultFile = tempFilePath("ring_stress_result.txt");
    const uint64_t count = 100000;

    std::remove(resultFile.c_str());

    intptr_t writerHandle = spawnHelper("writer", segmentName, count, resultFile);
    REQUIRE(writerHandle != -1);
    intptr_t readerHandle = spawnHelper("reader", segmentName, count, resultFile);
    REQUIRE(readerHandle != -1);

    int writerStatus = waitForExit(writerHandle);
    int readerStatus = waitForExit(readerHandle);

    std::ifstream in{resultFile};
    std::string line;
    std::getline(in, line);

    INFO("writer exit=" << writerStatus << " reader exit=" << readerStatus << " result=\"" << line << "\"");
    CHECK(writerStatus == 0);
    CHECK(readerStatus == 0);
    REQUIRE(line.substr(0, 2) == "OK");

    uint64_t received = std::stoull(line.substr(3));
    CHECK(received == count);
}

TEST_CASE("drop-on-full: tryPush never blocks and always counts a drop, single process", "[ring_buffer]") {
    BookDeltaRingBuffer ring;

    // Fill to capacity -- every one of these must succeed since nothing
    // has been popped yet.
    for (uint64_t i = 0; i < RING_BUFFER_CAPACITY; ++i) {
        BookDelta delta{};
        delta.seq = i;
        REQUIRE(ring.tryPush(delta));
    }
    REQUIRE(ring.droppedCount() == 0);

    auto start = std::chrono::steady_clock::now();
    constexpr uint64_t kOverflowAttempts = 5000;
    for (uint64_t i = 0; i < kOverflowAttempts; ++i) {
        BookDelta delta{};
        delta.seq = RING_BUFFER_CAPACITY + i;
        CHECK_FALSE(ring.tryPush(delta)); // full -- must return false, not overwrite
    }
    auto elapsed = std::chrono::steady_clock::now() - start;

    CHECK(ring.droppedCount() == kOverflowAttempts);
    // Never blocks: no reader exists at all in this test, so if tryPush
    // ever waited on one, this loop would never finish. A generous bound
    // (not a tight latency assertion) just confirms it returned
    // immediately rather than hanging.
    CHECK(std::chrono::duration_cast<std::chrono::seconds>(elapsed).count() < 5);
}

TEST_CASE("reset clears write/read indices and the drop counter", "[ring_buffer]") {
    BookDeltaRingBuffer ring;
    for (uint64_t i = 0; i < RING_BUFFER_CAPACITY + 10; ++i) {
        BookDelta delta{};
        delta.seq = i;
        ring.tryPush(delta); // some of these will overflow and increment droppedCount
    }
    REQUIRE(ring.droppedCount() > 0);

    ring.reset();

    CHECK(ring.droppedCount() == 0);
    // Fresh capacity is available again post-reset.
    BookDelta delta{};
    delta.seq = 1;
    CHECK(ring.tryPush(delta));
    BookDelta popped;
    CHECK(ring.tryPop(popped));
    CHECK(popped.seq == 1);
}
