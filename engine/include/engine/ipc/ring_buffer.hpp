#pragma once

#include <atomic>
#include <cstdint>
#include <cstring>
#include <stdexcept>
#include <string_view>
#include <type_traits>

#include "engine/book/order_book.hpp"
#include "engine/common/price_quantity.hpp"

constexpr uint64_t RING_BUFFER_CAPACITY = 4096;

struct BookDelta {
    char symbol[16];
    uint64_t seq;
    uint64_t engineTsNanos;
    uint8_t side;
    Price price;
    Quantity size;
};

static_assert(std::is_trivially_copyable_v<BookDelta>);

inline uint8_t toWireSide(BookSide side) {
    return static_cast<uint8_t>(side);
}

inline void setSymbol(BookDelta& delta, std::string_view symbol) {
    if (symbol.size() >= sizeof(delta.symbol)) {
        throw std::invalid_argument("BookDelta: symbol too long for fixed-width field");
    }
    std::memset(delta.symbol, 0, sizeof(delta.symbol));
    std::memcpy(delta.symbol, symbol.data(), symbol.size());
}


class BookDeltaRingBuffer {
public:

    BookDeltaRingBuffer() = default;

    BookDeltaRingBuffer(const BookDeltaRingBuffer&) = delete;
    BookDeltaRingBuffer& operator=(const BookDeltaRingBuffer&) = delete;

    void reset() {
        writeIndex_.store(0, std::memory_order_relaxed);
        readIndex_.store(0, std::memory_order_relaxed);
        droppedCount_.store(0, std::memory_order_relaxed);
    }

    // release stops compiler reordering here
    // cross process visibility is x86-64 TSO store->store ordering guarantee
    // there's no c++ acquire pairing the release because the reader is a separate Go process
    // this would not be safe on ARM

    // returns false and drops item if queue is full
    bool tryPush(const BookDelta& item) {
        uint64_t w = writeIndex_.load(std::memory_order_relaxed);
        uint64_t r = readIndex_.load(std::memory_order_acquire);
        if (w - r >= RING_BUFFER_CAPACITY) {
            droppedCount_.fetch_add(1, std::memory_order_relaxed);
            return false;
        }
        slots_[w % RING_BUFFER_CAPACITY] = item;
        writeIndex_.store(w + 1, std::memory_order_release);
        return true;
    }

    uint64_t droppedCount() const {
        return droppedCount_.load(std::memory_order_relaxed);
    }

    // returns false if queue is empty
    bool tryPop(BookDelta& out) {
        uint64_t r = readIndex_.load(std::memory_order_relaxed);
        uint64_t w = writeIndex_.load(std::memory_order_acquire);
        if (r == w) {
            return false;
        }
        out = slots_[r % RING_BUFFER_CAPACITY];
        readIndex_.store(r + 1, std::memory_order_release);
        return true;
    }

private:
    std::atomic<uint64_t> writeIndex_{0};
    std::atomic<uint64_t> readIndex_{0};
    std::atomic<uint64_t> droppedCount_{0};
    BookDelta slots_[RING_BUFFER_CAPACITY];
};

static_assert(std::atomic<uint64_t>::is_always_lock_free);

using BookDeltaQueue = BookDeltaRingBuffer;
