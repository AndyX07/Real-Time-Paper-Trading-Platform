#pragma once

#include <atomic>
#include <cstddef>
#include <cstdint>
#include <type_traits>

#include "engine/book/order_book.hpp"
#include "engine/book/price_level.hpp"

struct BookSnapshot {
    char symbol[16];
    uint64_t seq;
    uint64_t engineTsNanos;
    uint16_t numBidLevels;
    uint16_t numAskLevels;
    PriceLevel bids[BOOK_DEPTH];
    PriceLevel asks[BOOK_DEPTH];
};

static_assert(std::is_trivially_copyable_v<BookSnapshot>);

template <typename T>
class SeqlockSlot {
public:
    static_assert(std::is_trivially_copyable_v<T>);

    SeqlockSlot() = default;

    SeqlockSlot(const SeqlockSlot&) = delete;
    SeqlockSlot& operator=(const SeqlockSlot&) = delete;

    void reset() {
        version_.store(0, std::memory_order_relaxed);
        value_ = T{};
    }

    // release stops compiler reordering here
    // cross process visibility is x86-64 TSO store->store ordering guarantee
    // there's no c++ acquire pairing the release because the reader is a separate Go process
    // this would not be safe on ARM

    // when version is odd, write is in progress
    // when version is even, write is finished
    void write(const T& value) {
        version_.fetch_add(1, std::memory_order_relaxed);
        value_ = value;
        version_.fetch_add(1, std::memory_order_release);
    }

    T read() const {
        T result;
        uint32_t before;
        uint32_t after;
        do {
            before = version_.load(std::memory_order_acquire);
            while (before & 1) {
                before = version_.load(std::memory_order_acquire);
            }
            result = value_;
            after = version_.load(std::memory_order_acquire);
        } while (before != after);
        return result;
    }

private:
    std::atomic<uint32_t> version_{0};
    T value_{};
};

using SnapshotSlot = SeqlockSlot<BookSnapshot>;
