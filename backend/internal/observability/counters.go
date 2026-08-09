package observability

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

type Counter struct {
	value atomic.Uint64
}

func (c *Counter) Inc() { c.value.Add(1) }

func (c *Counter) Add(delta uint64) { c.value.Add(delta) }

func (c *Counter) Value() uint64 { return c.value.Load() }

type BookCounters struct {
	Resyncs         Counter
	ClientOverflows Counter

	mu              sync.Mutex
	droppedBySymbol map[string]uint64
}

func NewBookCounters() *BookCounters {
	return &BookCounters{droppedBySymbol: make(map[string]uint64)}
}

func (b *BookCounters) RecordResync(symbol string, dropped uint64) {
	b.Resyncs.Inc()
	b.mu.Lock()
	b.droppedBySymbol[symbol] = dropped
	b.mu.Unlock()
	slog.Warn("observability.book resync recorded", "symbol", symbol, "dropped", dropped)
}

func (b *BookCounters) RecordClientOverflow() {
	b.ClientOverflows.Inc()
	slog.Warn("observability.book client overflow recorded")
}

type BookCountersSnapshot struct {
	Resyncs         uint64            `json:"resyncs"`
	ClientOverflows uint64            `json:"clientOverflows"`
	DroppedBySymbol map[string]uint64 `json:"droppedBySymbol"`
}

func (b *BookCounters) Snapshot() BookCountersSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	dropped := make(map[string]uint64, len(b.droppedBySymbol))
	for symbol, count := range b.droppedBySymbol {
		dropped[symbol] = count
	}
	return BookCountersSnapshot{
		Resyncs:         b.Resyncs.Value(),
		ClientOverflows: b.ClientOverflows.Value(),
		DroppedBySymbol: dropped,
	}
}
