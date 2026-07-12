#include "engine/ipc/shared_memory_manager.hpp"

#include <cstring>
#include <stdexcept>

#include <boost/interprocess/exceptions.hpp>
#include <boost/interprocess/shared_memory_object.hpp>

namespace bip = boost::interprocess;

namespace {

constexpr const char* SEGMENT_NAME = "paper_trader_book_v1";
constexpr const char* LAYOUT_OBJECT_NAME = "layout";

// Payload is ~7MB for 32 symbol slots the rest is slack for boost::interprocess's own allocator bookkeeping.
constexpr size_t SEGMENT_BYTES = 10ull * 1024 * 1024;

void setSlotSymbol(SymbolSlot& slot, std::string_view symbol) {
    if (symbol.size() >= sizeof(slot.symbol)) {
        throw std::invalid_argument("SharedMemoryManager: symbol too long for fixed-width field");
    }
    std::memset(slot.symbol, 0, sizeof(slot.symbol));
    std::memcpy(slot.symbol, symbol.data(), symbol.size());
}

bool hasValidHeader(SharedMemorySegment* layout) {
    return layout != nullptr && layout->header.magic == SHARED_MEMORY_MAGIC && layout->header.version == SHARED_MEMORY_SEGMENT_VERSION;
}

}

SharedMemoryManager::SharedMemoryManager() {
    bool reuseExisting = false;

    try {
        bip::managed_shared_memory existing(bip::open_only, SEGMENT_NAME);
        reuseExisting = hasValidHeader(existing.find<SharedMemorySegment>(LAYOUT_OBJECT_NAME).first);
    } catch (const bip::interprocess_exception&) {

    }

    if (reuseExisting) {
        segment_ = bip::managed_shared_memory(bip::open_only, SEGMENT_NAME);
        layout_ = segment_.find<SharedMemorySegment>(LAYOUT_OBJECT_NAME).first;

        for (auto& slot : layout_->slots) {
            releaseSlot(&slot);
        }
        return;
    }

    bip::shared_memory_object::remove(SEGMENT_NAME);
    segment_ = bip::managed_shared_memory(bip::create_only, SEGMENT_NAME, SEGMENT_BYTES);
    layout_ = segment_.construct<SharedMemorySegment>(LAYOUT_OBJECT_NAME)();
    layout_->header.magic = SHARED_MEMORY_MAGIC;
    layout_->header.version = SHARED_MEMORY_SEGMENT_VERSION;
}

SymbolSlot* SharedMemoryManager::claimSlot(std::string_view symbol) {
    for (auto& slot : layout_->slots) {
        if (!slot.claimed.load(std::memory_order_acquire)) {
            slot.deltaQueue.reset();
            slot.snapshotSlot.reset();
            setSlotSymbol(slot, symbol);
            slot.claimed.store(true, std::memory_order_release);
            return &slot;
        }
    }
    return nullptr;
}

void SharedMemoryManager::releaseSlot(SymbolSlot* slot) {
    std::memset(slot->symbol, 0, sizeof(slot->symbol));
    slot->claimed.store(false, std::memory_order_release);
}
