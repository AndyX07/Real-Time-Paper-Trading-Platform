#pragma once

#include <cstddef>
#include <cstdint>
#include <string_view>
#include <type_traits>

#include <boost/interprocess/mapped_region.hpp>
#include <boost/interprocess/shared_memory_object.hpp>

#include "engine/ipc/ring_buffer.hpp"
#include "engine/ipc/snapshot_slot.hpp"

constexpr size_t SYMBOL_SLOT_POOL_SIZE = 32;

constexpr uint32_t SHARED_MEMORY_SEGMENT_VERSION = 1;
constexpr uint32_t SHARED_MEMORY_MAGIC = 0x50545253; // "PTRS" (Paper Trader Ring Segment)

struct SharedMemoryHeader {
    uint32_t magic;
    uint32_t version;
};

static_assert(std::is_trivially_copyable_v<SharedMemoryHeader>);

struct SymbolSlot {
    // uint32_t, not bool, so it lines up byte-for-byte with the Go side --
    // Go's sync/atomic has no LoadUint8, so the mirrored struct reads this as uint32.
    std::atomic<uint32_t> claimed{0};
    char symbol[16]{};
    BookDeltaRingBuffer deltaQueue;
    SnapshotSlot snapshotSlot;
};

struct SharedMemorySegment {
    SharedMemoryHeader header;
    SymbolSlot slots[SYMBOL_SLOT_POOL_SIZE];
};

class SharedMemoryManager {
public:
    explicit SharedMemoryManager(std::string_view segmentName = "paper_trader_book_v1");

    ~SharedMemoryManager() = default;

    SharedMemoryManager(const SharedMemoryManager&) = delete;
    SharedMemoryManager& operator=(const SharedMemoryManager&) = delete;

    SymbolSlot* claimSlot(std::string_view symbol);

    void releaseSlot(SymbolSlot* slot);

private:
    boost::interprocess::shared_memory_object segmentObject_;
    boost::interprocess::mapped_region region_;
    SharedMemorySegment* layout_ = nullptr;
};
