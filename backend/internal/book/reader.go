package book

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	mmap "github.com/edsrzf/mmap-go"

	"papertrader/backend/internal/observability"
)

var PollTick = 1 * time.Millisecond

// SegmentNameOverride lets tests point at an isolated shared-memory segment
// instead of the real SegmentName -- mirrors PollTick's test-seam pattern.
// Empty (the default) means "use SegmentName", unaffected in production.
var SegmentNameOverride string

type PriceLevelTicks struct {
	PriceTicks int64
	SizeTicks  int64
}

type DeltaEvent struct {
	Symbol     string
	Seq        uint64
	Side       string
	PriceTicks int64
	SizeTicks  int64
}

type SnapshotEvent struct {
	Symbol string
	Seq    uint64
	Bids   []PriceLevelTicks
	Asks   []PriceLevelTicks
}

type DeltaCallback func(DeltaEvent)
type SnapshotCallback func(SnapshotEvent)

type BookPoller struct {
	onDelta    DeltaCallback
	onSnapshot SnapshotCallback

	file    *os.File
	mm      mmap.MMap
	segment *SharedMemorySegment

	counters *observability.BookCounters

	mu           sync.Mutex
	slotBySymbol map[string]int
	expectedSeq  map[string]uint64
	bids         map[string]map[int64]int64
	asks         map[string]map[int64]int64

	stopCh chan struct{}
}

func NewBookPoller(onDelta DeltaCallback, onSnapshot SnapshotCallback) *BookPoller {
	return &BookPoller{
		onDelta:      onDelta,
		onSnapshot:   onSnapshot,
		counters:     observability.NewBookCounters(),
		slotBySymbol: make(map[string]int),
		expectedSeq:  make(map[string]uint64),
		bids:         make(map[string]map[int64]int64),
		asks:         make(map[string]map[int64]int64),
		stopCh:       make(chan struct{}),
	}
}

func (p *BookPoller) Counters() *observability.BookCounters {
	return p.counters
}

func segmentPath() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("book: could not determine source file path")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	segmentName := SegmentName
	if SegmentNameOverride != "" {
		segmentName = SegmentNameOverride
	}
	return filepath.Join(repoRoot, ".shm", segmentName), nil
}

func (p *BookPoller) attach() error {
	path, err := segmentPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("book.reader: no segment at %s -- is the engine running? (%w)", path, err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("book.reader: open %s: %w", path, err)
	}
	segSize := int(unsafe.Sizeof(SharedMemorySegment{}))
	m, err := mmap.MapRegion(f, segSize, mmap.RDWR, 0, 0)
	if err != nil {
		f.Close()
		return fmt.Errorf("book.reader: mmap %s: %w", path, err)
	}
	segment := (*SharedMemorySegment)(unsafe.Pointer(&m[0]))
	if segment.Header.Magic != SharedMemoryMagic || segment.Header.Version != SharedMemorySegmentVersion {
		m.Unmap()
		f.Close()
		return fmt.Errorf("book.reader: segment header mismatch -- engine/backend version skew?")
	}
	p.file = f
	p.mm = m
	p.segment = segment
	slog.Info("book.reader attached", "path", path, "bytes", segSize)
	return nil
}

func (p *BookPoller) ensureAttached() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.segment != nil {
		return nil
	}
	return p.attach()
}

func matchesSymbol(field []byte, wanted string) bool {
	n := 0
	for n < len(field) && field[n] != 0 {
		n++
	}
	return string(field[:n]) == wanted
}

func (p *BookPoller) findSlot(ctx context.Context, symbol string) (int, error) {
	for range 50 {
		for idx := range p.segment.Slots {
			slot := &p.segment.Slots[idx]
			if slot.Claimed != 0 && matchesSymbol(slot.Symbol[:], symbol) {
				return idx, nil
			}
		}
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return -1, fmt.Errorf("book.reader: no claimed slot found for %s", symbol)
}

func trimNulIndex(b []byte) int {
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return n
}

func (p *BookPoller) readSnapshot(slot *SnapshotSlot) (SnapshotEvent, error) {
	for range 1000 {
		before := atomic.LoadUint32(&slot.Version)
		if before%2 == 1 {
			continue
		}
		value := slot.Value
		after := atomic.LoadUint32(&slot.Version)
		if before == after {
			symbol := string(value.Symbol[:trimNulIndex(value.Symbol[:])])
			bids := make([]PriceLevelTicks, value.NumBidLevels)
			for i := range bids {
				bids[i] = PriceLevelTicks{PriceTicks: value.Bids[i].Price.Ticks, SizeTicks: value.Bids[i].Quantity.Ticks}
			}
			asks := make([]PriceLevelTicks, value.NumAskLevels)
			for i := range asks {
				asks[i] = PriceLevelTicks{PriceTicks: value.Asks[i].Price.Ticks, SizeTicks: value.Asks[i].Quantity.Ticks}
			}
			return SnapshotEvent{Symbol: symbol, Seq: value.Seq, Bids: bids, Asks: asks}, nil
		}
	}
	return SnapshotEvent{}, fmt.Errorf("book.reader: seqlock read did not stabilize after 1000 attempts")
}

func (p *BookPoller) awaitInitialSnapshot(ctx context.Context, symbol string, slot *SnapshotSlot) (SnapshotEvent, error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := p.readSnapshot(slot)
		if err != nil {
			return SnapshotEvent{}, err
		}
		if snapshot.Seq > 0 {
			return snapshot, nil
		}
		select {
		case <-ctx.Done():
			return SnapshotEvent{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	slog.Warn("book.reader: no snapshot yet after 30s, seeding an empty book", "symbol", symbol)
	return p.readSnapshot(slot)
}

func (p *BookPoller) rebuildMirrorLocked(symbol string, snapshot SnapshotEvent) {
	bids := make(map[int64]int64, len(snapshot.Bids))
	for _, l := range snapshot.Bids {
		bids[l.PriceTicks] = l.SizeTicks
	}
	asks := make(map[int64]int64, len(snapshot.Asks))
	for _, l := range snapshot.Asks {
		asks[l.PriceTicks] = l.SizeTicks
	}
	p.bids[symbol] = bids
	p.asks[symbol] = asks
}

func (p *BookPoller) applyToMirrorLocked(symbol, side string, priceTicks, sizeTicks int64) {
	side_ := p.bids[symbol]
	if side == "ask" {
		side_ = p.asks[symbol]
	}
	if sizeTicks == 0 {
		delete(side_, priceTicks)
	} else {
		side_[priceTicks] = sizeTicks
	}
}

func tryPop(queue *BookDeltaRingBuffer) (DeltaEvent, bool) {
	r := atomic.LoadUint64(&queue.ReadIndex)
	w := atomic.LoadUint64(&queue.WriteIndex)
	if r == w {
		return DeltaEvent{}, false
	}
	item := queue.Slots[r%RingBufferCapacity]
	side := "bid"
	if item.Side != 0 {
		side = "ask"
	}
	atomic.StoreUint64(&queue.ReadIndex, r+1)
	return DeltaEvent{Seq: item.Seq, Side: side, PriceTicks: item.Price.Ticks, SizeTicks: item.Size.Ticks}, true
}

func (p *BookPoller) catchUpFromSnapshot(symbol string, queue *BookDeltaRingBuffer, snapshot SnapshotEvent) {
	expected := snapshot.Seq + 1
	for {
		item, ok := tryPop(queue)
		if !ok {
			break
		}
		item.Symbol = symbol
		if item.Seq <= snapshot.Seq {
			continue // already reflected in the snapshot just read
		}
		if item.Seq != expected {
			slog.Warn("book.reader: jump while catching up from snapshot (dropped entries, accepting new baseline)",
				"symbol", symbol, "expected", expected, "got", item.Seq)
		}
		p.mu.Lock()
		p.applyToMirrorLocked(symbol, item.Side, item.PriceTicks, item.SizeTicks)
		p.mu.Unlock()
		p.onDelta(item)
		expected = item.Seq + 1
	}
	p.mu.Lock()
	p.expectedSeq[symbol] = expected
	p.mu.Unlock()
}

func mapToLevels(m map[int64]int64, descending bool) []PriceLevelTicks {
	levels := make([]PriceLevelTicks, 0, len(m))
	for price, size := range m {
		levels = append(levels, PriceLevelTicks{PriceTicks: price, SizeTicks: size})
	}
	sort.Slice(levels, func(i, j int) bool {
		if descending {
			return levels[i].PriceTicks > levels[j].PriceTicks
		}
		return levels[i].PriceTicks < levels[j].PriceTicks
	})
	return levels
}

func (p *BookPoller) currentSnapshotLocked(symbol string) *SnapshotEvent {
	bids, ok := p.bids[symbol]
	if !ok {
		return nil
	}
	asks := p.asks[symbol]
	seq := p.expectedSeq[symbol] - 1
	event := SnapshotEvent{
		Symbol: symbol,
		Seq:    seq,
		Bids:   mapToLevels(bids, true),  // descending -- best (highest) bid first
		Asks:   mapToLevels(asks, false), // ascending -- best (lowest) ask first
	}
	return &event
}

func (p *BookPoller) RegisterSymbol(ctx context.Context, symbol string) (SnapshotEvent, error) {
	if err := p.ensureAttached(); err != nil {
		return SnapshotEvent{}, err
	}
	idx, err := p.findSlot(ctx, symbol)
	if err != nil {
		return SnapshotEvent{}, err
	}
	slot := &p.segment.Slots[idx]

	snapshot, err := p.awaitInitialSnapshot(ctx, symbol, &slot.Snapshot)
	if err != nil {
		return SnapshotEvent{}, err
	}

	p.mu.Lock()
	p.rebuildMirrorLocked(symbol, snapshot)
	p.mu.Unlock()

	p.catchUpFromSnapshot(symbol, &slot.DeltaQueue, snapshot)

	p.mu.Lock()
	p.slotBySymbol[symbol] = idx
	current := p.currentSnapshotLocked(symbol)
	p.mu.Unlock()

	if current != nil {
		return *current, nil
	}
	return snapshot, nil
}

func (p *BookPoller) UnregisterSymbol(symbol string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.slotBySymbol, symbol)
	delete(p.expectedSeq, symbol)
	delete(p.bids, symbol)
	delete(p.asks, symbol)
}

func (p *BookPoller) CurrentSnapshot(symbol string) *SnapshotEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentSnapshotLocked(symbol)
}

func (p *BookPoller) resync(symbol string, slot *SymbolSlot) {
	snapshot, err := p.readSnapshot(&slot.Snapshot)
	if err != nil {
		slog.Error("book.reader: resync snapshot read failed", "symbol", symbol, "error", err)
		return
	}
	p.mu.Lock()
	haveSeq := p.expectedSeq[symbol]
	if snapshot.Seq+1 > haveSeq {
		p.rebuildMirrorLocked(symbol, snapshot)
	} else {
		snapshot.Seq = haveSeq - 1
	}
	p.mu.Unlock()

	p.catchUpFromSnapshot(symbol, &slot.DeltaQueue, snapshot)

	p.mu.Lock()
	current := p.currentSnapshotLocked(symbol)
	p.mu.Unlock()
	if current != nil {
		p.onSnapshot(*current)
	} else {
		p.onSnapshot(snapshot)
	}
}

func (p *BookPoller) pollSymbol(symbol string, slot *SymbolSlot) {
	queue := &slot.DeltaQueue
	for {
		item, ok := tryPop(queue)
		if !ok {
			return
		}
		item.Symbol = symbol // tryPop doesn't know the symbol; it's threaded from here, not re-read from the ring buffer entry

		p.mu.Lock()
		expected := p.expectedSeq[symbol]
		if item.Seq != expected {
			p.mu.Unlock()
			dropped := atomic.LoadUint64(&queue.DroppedCount)
			slog.Warn("book.reader: seq gap, resyncing",
				"symbol", symbol, "expected", expected, "got", item.Seq,
				"dropped", dropped)
			p.counters.RecordResync(symbol, dropped)
			p.resync(symbol, slot)
			return
		}
		p.applyToMirrorLocked(symbol, item.Side, item.PriceTicks, item.SizeTicks)
		p.expectedSeq[symbol] = item.Seq + 1
		p.mu.Unlock()

		p.onDelta(item)
	}
}

func (p *BookPoller) pollOnce() {
	p.mu.Lock()
	symbols := make(map[string]int, len(p.slotBySymbol))
	maps.Copy(symbols, p.slotBySymbol)
	p.mu.Unlock()

	for symbol, idx := range symbols {
		p.pollSymbol(symbol, &p.segment.Slots[idx])
	}
}

func (p *BookPoller) Run(ctx context.Context) error {
	if err := p.ensureAttached(); err != nil {
		return err
	}
	defer func() {
		if p.mm != nil {
			p.mm.Unmap()
		}
		if p.file != nil {
			p.file.Close()
		}
	}()

	ticker := time.NewTicker(PollTick)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return nil
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			p.pollOnce()
		}
	}
}

func (p *BookPoller) Stop() {
	close(p.stopCh)
}
