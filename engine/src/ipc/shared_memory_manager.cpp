#include "engine/ipc/shared_memory_manager.hpp"

#include <cstring>
#include <new>
#include <stdexcept>

#include <boost/interprocess/exceptions.hpp>

namespace bip = boost::interprocess;

namespace {

constexpr const char* SEGMENT_NAME = "paper_trader_book_v1";
constexpr size_t SEGMENT_BYTES = sizeof(SharedMemorySegment);

void setSlotSymbol(SymbolSlot& slot, std::string_view symbol) {
    if (symbol.size() >= sizeof(slot.symbol)) {
        throw std::invalid_argument("SharedMemoryManager: symbol too long for fixed-width field");
    }
    std::memset(slot.symbol, 0, sizeof(slot.symbol));
    std::memcpy(slot.symbol, symbol.data(), symbol.size());
}

bool hasValidHeader(const SharedMemorySegment* layout, size_t mappedBytes) {
    return layout != nullptr && mappedBytes >= sizeof(SharedMemorySegment) &&
           layout->header.magic == SHARED_MEMORY_MAGIC && layout->header.version == SHARED_MEMORY_SEGMENT_VERSION;
}

}

SharedMemoryManager::SharedMemoryManager() {
    bool reuseExisting = false;

    try {
        bip::shared_memory_object existing(bip::open_only, SEGMENT_NAME, bip::read_write);
        bip::mapped_region existingRegion(existing, bip::read_write);
        reuseExisting = hasValidHeader(static_cast<const SharedMemorySegment*>(existingRegion.get_address()),
                                        existingRegion.get_size());
    } catch (const bip::interprocess_exception&) {
    }

    if (reuseExisting) {
        segmentObject_ = bip::shared_memory_object(bip::open_only, SEGMENT_NAME, bip::read_write);
        region_ = bip::mapped_region(segmentObject_, bip::read_write);
        layout_ = static_cast<SharedMemorySegment*>(region_.get_address());

        for (auto& slot : layout_->slots) {
            releaseSlot(&slot);
        }
        return;
    }

    bip::shared_memory_object::remove(SEGMENT_NAME);
    segmentObject_ = bip::shared_memory_object(bip::create_only, SEGMENT_NAME, bip::read_write);
    segmentObject_.truncate(static_cast<bip::offset_t>(SEGMENT_BYTES));
    region_ = bip::mapped_region(segmentObject_, bip::read_write);
    layout_ = new (region_.get_address()) SharedMemorySegment();
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
