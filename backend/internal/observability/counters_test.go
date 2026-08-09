package observability

import (
	"sync"
	"testing"
)

func TestCounterIncAndAdd(t *testing.T) {
	var c Counter
	c.Inc()
	c.Inc()
	c.Add(3)
	if got := c.Value(); got != 5 {
		t.Fatalf("Value() = %d, want 5", got)
	}
}

func TestCounterConcurrent(t *testing.T) {
	var c Counter
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Inc()
		}()
	}
	wg.Wait()
	if got := c.Value(); got != 100 {
		t.Fatalf("Value() = %d, want 100", got)
	}
}

func TestBookCountersRecordResync(t *testing.T) {
	b := NewBookCounters()
	b.RecordResync("BTC/USD", 7)
	b.RecordResync("BTC/USD", 12)
	b.RecordResync("ETH/USD", 3)

	snap := b.Snapshot()
	if snap.Resyncs != 3 {
		t.Fatalf("Resyncs = %d, want 3", snap.Resyncs)
	}
	if snap.DroppedBySymbol["BTC/USD"] != 12 {
		t.Fatalf("DroppedBySymbol[BTC/USD] = %d, want 12 (last-observed, not summed)", snap.DroppedBySymbol["BTC/USD"])
	}
	if snap.DroppedBySymbol["ETH/USD"] != 3 {
		t.Fatalf("DroppedBySymbol[ETH/USD] = %d, want 3", snap.DroppedBySymbol["ETH/USD"])
	}
}

func TestBookCountersRecordClientOverflow(t *testing.T) {
	b := NewBookCounters()
	b.RecordClientOverflow()
	b.RecordClientOverflow()

	if got := b.Snapshot().ClientOverflows; got != 2 {
		t.Fatalf("ClientOverflows = %d, want 2", got)
	}
}

func TestBookCountersSnapshotIsolated(t *testing.T) {
	b := NewBookCounters()
	b.RecordResync("BTC/USD", 1)

	snap := b.Snapshot()
	snap.DroppedBySymbol["BTC/USD"] = 999

	if got := b.Snapshot().DroppedBySymbol["BTC/USD"]; got != 1 {
		t.Fatalf("mutating a returned snapshot leaked into internal state: got %d, want 1", got)
	}
}
