package book

import (
	"testing"
	"unsafe"
)

func TestBookDeltaLayoutMatchesCpp(t *testing.T) {
	var d BookDelta

	checks := []struct {
		name   string
		offset uintptr
		want   uintptr
	}{
		{"Symbol", unsafe.Offsetof(d.Symbol), 0},
		{"Seq", unsafe.Offsetof(d.Seq), 16},
		{"EngineTsNanos", unsafe.Offsetof(d.EngineTsNanos), 24},
		{"Side", unsafe.Offsetof(d.Side), 32},
		{"Price", unsafe.Offsetof(d.Price), 40}, // 7-byte hole after Side for Price's 8-byte alignment
		{"Size", unsafe.Offsetof(d.Size), 48},
	}
	for _, c := range checks {
		if c.offset != c.want {
			t.Errorf("BookDelta.%s offset = %d, want %d (does this match ring_buffer.hpp's BookDelta?)", c.name, c.offset, c.want)
		}
	}
	if got, want := unsafe.Sizeof(d), uintptr(56); got != want {
		t.Errorf("sizeof(BookDelta) = %d, want %d", got, want)
	}
}

func TestBookDeltaRingBufferWriteIndexIsFirst(t *testing.T) {
	var q BookDeltaRingBuffer
	if off := unsafe.Offsetof(q.WriteIndex); off != 0 {
		t.Errorf("BookDeltaRingBuffer.WriteIndex offset = %d, want 0 -- tryPop reads ReadIndex before WriteIndex, "+
			"which is fine, but the two mirrored structs must agree on field order or shared memory silently desyncs", off)
	}
}

func TestBookSnapshotLayoutMatchesCpp(t *testing.T) {
	var s BookSnapshot

	const priceLevelSize = unsafe.Sizeof(PriceLevel{})
	if priceLevelSize != 16 {
		t.Fatalf("sizeof(PriceLevel) = %d, want 16", priceLevelSize)
	}
	bidsOffset := unsafe.Offsetof(s.Bids)
	wantAsksOffset := bidsOffset + priceLevelSize*BookDepth
	wantSize := wantAsksOffset + priceLevelSize*BookDepth

	checks := []struct {
		name   string
		offset uintptr
		want   uintptr
	}{
		{"Symbol", unsafe.Offsetof(s.Symbol), 0},
		{"Seq", unsafe.Offsetof(s.Seq), 16},
		{"EngineTsNanos", unsafe.Offsetof(s.EngineTsNanos), 24},
		{"NumBidLevels", unsafe.Offsetof(s.NumBidLevels), 32},
		{"NumAskLevels", unsafe.Offsetof(s.NumAskLevels), 34},
		{"Bids", bidsOffset, 40}, // 4-byte hole after NumAskLevels for PriceLevel's 8-byte alignment
		{"Asks", unsafe.Offsetof(s.Asks), wantAsksOffset},
	}
	for _, c := range checks {
		if c.offset != c.want {
			t.Errorf("BookSnapshot.%s offset = %d, want %d (does this match snapshot_slot.hpp's BookSnapshot?)", c.name, c.offset, c.want)
		}
	}
	if got := unsafe.Sizeof(s); got != wantSize {
		t.Errorf("sizeof(BookSnapshot) = %d, want %d", got, wantSize)
	}
}

func TestSnapshotSlotVersionIsFirst(t *testing.T) {
	var s SnapshotSlot
	if off := unsafe.Offsetof(s.Version); off != 0 {
		t.Errorf("SnapshotSlot.Version offset = %d, want 0 -- must match the field order SeqlockSlot<BookSnapshot> assumes in C++", off)
	}
	if off := unsafe.Offsetof(s.Value); off != 8 {
		t.Errorf("SnapshotSlot.Value offset = %d, want 8 (4-byte hole after the uint32 Version for BookSnapshot's 8-byte alignment)", off)
	}
}
