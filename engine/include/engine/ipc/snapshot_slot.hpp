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

// Pins the hand-mirrored Go layout (backend/internal/book/layout.go) byte-for-byte.
// If this ever trips, the Go struct needs the matching change too.
static_assert(offsetof(BookSnapshot, symbol) == 0);
static_assert(offsetof(BookSnapshot, seq) == 16);
static_assert(offsetof(BookSnapshot, engineTsNanos) == 24);
static_assert(offsetof(BookSnapshot, numBidLevels) == 32);
static_assert(offsetof(BookSnapshot, numAskLevels) == 34);
static_assert(offsetof(BookSnapshot, bids) == 40); // 4-byte hole after numAskLevels for PriceLevel's 8-byte alignment
static_assert(offsetof(BookSnapshot, asks) == 40 + sizeof(PriceLevel) * BOOK_DEPTH);
static_assert(sizeof(BookSnapshot) == 40 + sizeof(PriceLevel) * BOOK_DEPTH * 2);
static_assert(sizeof(PriceLevel) == 16);

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

    // when version is odd, write is in progress
    // when version is even, write is finished
    //
    // the odd bump needs acquire: it must stop the payload write below from being
    // hoisted above it (a constraint on the *subsequent* access). relaxed would leave
    // the compiler free to reorder value_ = value ahead of the version bump.
    void write(const T& value) {
        version_.fetch_add(1, std::memory_order_acq_rel);
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

public:
    static constexpr size_t versionOffset() { return offsetof(SeqlockSlot, version_); }
    static constexpr size_t valueOffset() { return offsetof(SeqlockSlot, value_); }

private:
    std::atomic<uint32_t> version_{0};
    T value_{};
};

using SnapshotSlot = SeqlockSlot<BookSnapshot>;

static_assert(SnapshotSlot::versionOffset() == 0);
static_assert(SnapshotSlot::valueOffset() == 8); // 4-byte hole after the uint32_t version for BookSnapshot's 8-byte alignment
