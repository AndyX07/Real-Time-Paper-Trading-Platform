#include "engine/ipc/shared_memory_manager.hpp"

#include <cstring>
#include <filesystem>
#include <new>
#include <stdexcept>
#include <string>

#include <boost/interprocess/exceptions.hpp>

namespace bip = boost::interprocess;

namespace {

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

SharedMemoryManager::SharedMemoryManager(std::string_view segmentName) {
    std::string name{segmentName};

    // BOOST_INTERPROCESS_SHARED_DIR_PATH skips Boost's own directory-creation
    // step (it only runs that for its default, boot-timestamped location).
    std::filesystem::create_directories(BOOST_INTERPROCESS_SHARED_DIR_PATH);

    bool reuseExisting = false;

    try {
        bip::shared_memory_object existing{bip::open_only, name.c_str(), bip::read_write};
        bip::mapped_region existingRegion{existing, bip::read_write};
        reuseExisting = hasValidHeader(static_cast<const SharedMemorySegment*>(existingRegion.get_address()),
                                        existingRegion.get_size());
    } catch (const bip::interprocess_exception&) {
    }

    if (reuseExisting) {
        segmentObject_ = bip::shared_memory_object(bip::open_only, name.c_str(), bip::read_write);
        region_ = bip::mapped_region(segmentObject_, bip::read_write);
        layout_ = static_cast<SharedMemorySegment*>(region_.get_address());

        for (auto& slot : layout_->slots) {
            releaseSlot(&slot);
        }
        return;
    }

    bip::shared_memory_object::remove(name.c_str());
    segmentObject_ = bip::shared_memory_object(bip::create_only, name.c_str(), bip::read_write);
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
            slot.claimed.store(1, std::memory_order_release);
            return &slot;
        }
    }
    return nullptr;
}

void SharedMemoryManager::releaseSlot(SymbolSlot* slot) {
    std::memset(slot->symbol, 0, sizeof(slot->symbol));
    slot->claimed.store(0, std::memory_order_release);
}
