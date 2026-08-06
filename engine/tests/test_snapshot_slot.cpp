#include <catch2/catch_test_macros.hpp>

#include <atomic>
#include <chrono>
#include <thread>
#include <vector>

#include "engine/ipc/snapshot_slot.hpp"

namespace {

// Every field is a deterministic function of one monotonic counter --
// any single read() result must be internally consistent with exactly
// one counter value, never a mix of two.
BookSnapshot snapshotFor(uint64_t counter) {
    BookSnapshot snap{};
    snap.seq = counter;
    snap.engineTsNanos = counter * 3;
    snap.numBidLevels = 1;
    snap.numAskLevels = 1;
    snap.bids[0] = PriceLevel{Price{static_cast<int64_t>(counter) * 7}, Quantity{static_cast<int64_t>(counter) * 11}};
    snap.asks[0] = PriceLevel{Price{static_cast<int64_t>(counter) * 13}, Quantity{static_cast<int64_t>(counter) * 17}};
    return snap;
}

bool isConsistent(const BookSnapshot& snap) {
    uint64_t counter = snap.seq;
    return snap.engineTsNanos == counter * 3 && snap.numBidLevels == 1 && snap.numAskLevels == 1 &&
           snap.bids[0].price.ticks == static_cast<int64_t>(counter) * 7 &&
           snap.bids[0].quantity.ticks == static_cast<int64_t>(counter) * 11 &&
           snap.asks[0].price.ticks == static_cast<int64_t>(counter) * 13 &&
           snap.asks[0].quantity.ticks == static_cast<int64_t>(counter) * 17;
}

}

TEST_CASE("no reader ever observes a torn BookSnapshot under sustained concurrent writes", "[snapshot_slot]") {
    SnapshotSlot slot;
    std::atomic<bool> stop{false};
    std::atomic<uint64_t> readsChecked{0};
    std::atomic<bool> tornReadDetected{false};

    std::thread writer{[&] {
        uint64_t counter = 1;
        while (!stop.load(std::memory_order_relaxed)) {
            slot.write(snapshotFor(counter));
            ++counter;
        }
    }};

    constexpr int kReaderCount = 8;
    std::vector<std::thread> readers;
    for (int i = 0; i < kReaderCount; ++i) {
        readers.emplace_back([&] {
            while (!stop.load(std::memory_order_relaxed)) {
                BookSnapshot snap = slot.read();
                if (snap.seq == 0) {
                    continue; // before the writer's first publish
                }
                if (!isConsistent(snap)) {
                    tornReadDetected.store(true, std::memory_order_relaxed);
                }
                readsChecked.fetch_add(1, std::memory_order_relaxed);
            }
        });
    }

    std::this_thread::sleep_for(std::chrono::milliseconds{500});
    stop.store(true, std::memory_order_relaxed);

    writer.join();
    for (auto& r : readers) {
        r.join();
    }

    CHECK_FALSE(tornReadDetected.load());
    CHECK(readsChecked.load() > 0); // sanity: readers actually observed something
}

TEST_CASE("readers never block indefinitely even under maximum writer contention", "[snapshot_slot]") {
    SnapshotSlot slot;
    std::atomic<bool> stop{false};

    std::thread writer{[&] {
        uint64_t counter = 1;
        while (!stop.load(std::memory_order_relaxed)) {
            slot.write(snapshotFor(counter));
            ++counter;
        }
    }};

    // Each read() must return promptly -- a livelocked reader would never
    // reach the iteration budget below within any reasonable time.
    constexpr int kIterations = 10000;
    auto start = std::chrono::steady_clock::now();
    for (int i = 0; i < kIterations; ++i) {
        (void)slot.read();
    }
    auto elapsed = std::chrono::steady_clock::now() - start;

    stop.store(true, std::memory_order_relaxed);
    writer.join();

    CHECK(std::chrono::duration_cast<std::chrono::seconds>(elapsed).count() < 10);
}

TEST_CASE("reset zero-initializes the slot", "[snapshot_slot]") {
    SnapshotSlot slot;
    slot.write(snapshotFor(42));
    REQUIRE(slot.read().seq == 42);

    slot.reset();

    BookSnapshot cleared = slot.read();
    CHECK(cleared.seq == 0);
    CHECK(cleared.numBidLevels == 0);
}
