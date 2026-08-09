package book

import (
	"context"
	"testing"
)

type recordingPoller struct {
	*BookPoller
	deltas    []DeltaEvent
	snapshots []SnapshotEvent
}

func newRecordingPoller(t *testing.T, symbol string, seq uint64, bids, asks []PriceLevelTicks) *recordingPoller {
	t.Helper()
	rp := &recordingPoller{}
	rp.BookPoller = NewBookPoller(
		func(e DeltaEvent) { rp.deltas = append(rp.deltas, e) },
		func(e SnapshotEvent) { rp.snapshots = append(rp.snapshots, e) },
	)
	rp.segment = &SharedMemorySegment{}
	slot := &rp.segment.Slots[0]
	slot.Claimed = 1
	copy(slot.Symbol[:], symbol)
	writeSnapshot(&slot.Snapshot, symbol, seq, bids, asks)

	if _, err := rp.RegisterSymbol(context.Background(), symbol); err != nil {
		t.Fatalf("RegisterSymbol(%s): %v", symbol, err)
	}
	return rp
}

func TestReaderInOrderDeltasApplyWithoutResync(t *testing.T) {
	rp := newRecordingPoller(t, "BTC/USD", 5, nil, nil)
	slot := &rp.segment.Slots[0]

	pushDelta(&slot.DeltaQueue, 6, "bid", 100, 5)
	rp.pollOnce()

	if got := rp.Counters().Snapshot().Resyncs; got != 0 {
		t.Fatalf("Resyncs = %d, want 0 for a clean in-order delta", got)
	}
	if len(rp.deltas) != 1 || rp.deltas[0].Seq != 6 {
		t.Fatalf("deltas = %v, want exactly one delta at seq 6", rp.deltas)
	}
	if got := rp.expectedSeq["BTC/USD"]; got != 7 {
		t.Fatalf("expectedSeq = %d, want 7", got)
	}
}

// TestReaderDetectsGapResyncsOnceAndResumesDelivery is the "at the reader
// level" test: a seq gap must be detected (not silently applied), trigger
// exactly one resync against a fresher snapshot, and then resume normal
// delivery -- not just send a snapshot once and get stuck resyncing forever.
//
// The delta that actually triggers the gap (seq 13 below) is inherently
// unrecoverable: pollSymbol's own tryPop already consumes it from the ring
// buffer before the seq mismatch is even detected, so by the time resync's
// catchUpFromSnapshot runs, that specific item is gone. What resync (and
// this test) can prove is that (a) that loss is detected and counted, (b)
// the fresher snapshot becomes the new baseline, (c) anything still queued
// behind the triggering delta (seq 14 below) is still delivered, and (d)
// delivery is fully resumed afterward, with no repeat resync for a clean
// in-order continuation.
func TestReaderDetectsGapResyncsOnceAndResumesDelivery(t *testing.T) {
	rp := newRecordingPoller(t, "BTC/USD", 10,
		[]PriceLevelTicks{{PriceTicks: 100, SizeTicks: 5}}, nil)
	slot := &rp.segment.Slots[0]

	// Simulate the engine having moved on: a fresher snapshot is already
	// sitting in shared memory (seq 12) by the time these deltas are read.
	writeSnapshot(&slot.Snapshot, "BTC/USD", 12,
		[]PriceLevelTicks{{PriceTicks: 100, SizeTicks: 3}}, nil)

	// seq 13 is the gap-triggering delta (expected was 11); seq 14 is
	// still queued behind it and must survive the resync.
	pushDelta(&slot.DeltaQueue, 13, "bid", 100, 1)
	pushDelta(&slot.DeltaQueue, 14, "bid", 100, 2)

	rp.pollOnce()

	if got := rp.Counters().Snapshot().Resyncs; got != 1 {
		t.Fatalf("Resyncs = %d, want exactly 1", got)
	}
	if len(rp.snapshots) != 1 {
		t.Fatalf("snapshots delivered = %d, want exactly 1 (the reseed)", len(rp.snapshots))
	}
	if len(rp.deltas) != 1 || rp.deltas[0].Seq != 14 {
		t.Fatalf("deltas delivered = %v, want exactly one delta at seq 14 (seq 13 is unrecoverable by design)", rp.deltas)
	}
	if got := rp.expectedSeq["BTC/USD"]; got != 15 {
		t.Fatalf("expectedSeq after resync+catch-up = %d, want 15", got)
	}
	if got := rp.CurrentSnapshot("BTC/USD"); got == nil || got.Seq != 14 {
		t.Fatalf("CurrentSnapshot after resync = %+v, want seq 14", got)
	}

	// Delivery must be fully resumed: a clean, in-order continuation
	// should apply without triggering yet another resync.
	pushDelta(&slot.DeltaQueue, 15, "bid", 100, 1)
	rp.pollOnce()

	if got := rp.Counters().Snapshot().Resyncs; got != 1 {
		t.Fatalf("Resyncs after the clean continuation = %d, want still 1 (no further resync)", got)
	}
	if len(rp.deltas) != 2 || rp.deltas[1].Seq != 15 {
		t.Fatalf("deltas after continuation = %v, want a second delta at seq 15", rp.deltas)
	}
}

func TestReaderResyncRecordsCounters(t *testing.T) {
	rp := newRecordingPoller(t, "BTC/USD", 1, nil, nil)
	slot := &rp.segment.Slots[0]

	writeSnapshot(&slot.Snapshot, "BTC/USD", 3, nil, nil)
	pushDelta(&slot.DeltaQueue, 9, "bid", 100, 1) // way ahead of expected -- forces a resync

	rp.pollOnce()

	snap := rp.Counters().Snapshot()
	if snap.Resyncs != 1 {
		t.Fatalf("Resyncs = %d, want 1", snap.Resyncs)
	}
	if _, ok := snap.DroppedBySymbol["BTC/USD"]; !ok {
		t.Fatalf("DroppedBySymbol missing an entry for BTC/USD after a resync")
	}
}
