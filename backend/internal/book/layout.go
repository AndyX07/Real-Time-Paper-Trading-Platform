package book

const (
	RingBufferCapacity         = 4096
	BookDepth                  = 100
	SymbolSlotPoolSize         = 32
	SharedMemoryMagic          = 0x50545253
	SharedMemorySegmentVersion = 1
	SegmentName                = "paper_trader_book_v1"
)

type Price struct {
	Ticks int64
}

type Quantity struct {
	Ticks int64
}

type PriceLevel struct {
	Price    Price
	Quantity Quantity
}

type BookDelta struct {
	Symbol        [16]byte
	Seq           uint64
	EngineTsNanos uint64
	Side          uint8
	Price         Price
	Size          Quantity
}

type BookDeltaRingBuffer struct {
	WriteIndex   uint64
	ReadIndex    uint64
	DroppedCount uint64
	Slots        [RingBufferCapacity]BookDelta
}

type BookSnapshot struct {
	Symbol        [16]byte
	Seq           uint64
	EngineTsNanos uint64
	NumBidLevels  uint16
	NumAskLevels  uint16
	Bids          [BookDepth]PriceLevel
	Asks          [BookDepth]PriceLevel
}

type SnapshotSlot struct {
	Version uint32
	Value   BookSnapshot
}

type SymbolSlot struct {
	Claimed    uint32 // atomic -- uint32 because Go's sync/atomic has no LoadUint8
	Symbol     [16]byte
	DeltaQueue BookDeltaRingBuffer
	Snapshot   SnapshotSlot
}

type SharedMemoryHeader struct {
	Magic   uint32
	Version uint32
}

type SharedMemorySegment struct {
	Header SharedMemoryHeader
	Slots  [SymbolSlotPoolSize]SymbolSlot
}
