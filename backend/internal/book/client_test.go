package book

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"

	"papertrader/backend/internal/schemas"
)

// newSeededPoller builds a BookPoller with an in-memory segment (no real
// mmap file), claims slot 0 for symbol, seqlock-writes an initial snapshot
// at seq, and runs it through RegisterSymbol so expectedSeq/the bid/ask
// mirror are seeded exactly as they would be for a real subscriber. seq
// must be >= 1 -- awaitInitialSnapshot only returns immediately for a
// snapshot with Seq > 0, otherwise it polls for up to 30s.
func newSeededPoller(t *testing.T, symbol string, seq uint64, bids, asks []PriceLevelTicks) *BookPoller {
	t.Helper()
	poller := NewBookPoller(func(DeltaEvent) {}, func(SnapshotEvent) {})
	poller.segment = &SharedMemorySegment{}
	slot := &poller.segment.Slots[0]
	slot.Claimed = 1
	copy(slot.Symbol[:], symbol)
	writeSnapshot(&slot.Snapshot, symbol, seq, bids, asks)

	if _, err := poller.RegisterSymbol(context.Background(), symbol); err != nil {
		t.Fatalf("RegisterSymbol(%s): %v", symbol, err)
	}
	return poller
}

// writeSnapshot performs a real seqlock write (odd -> write -> even),
// matching the protocol readSnapshot expects.
func writeSnapshot(slot *SnapshotSlot, symbol string, seq uint64, bids, asks []PriceLevelTicks) {
	atomic.AddUint32(&slot.Version, 1)
	var value BookSnapshot
	copy(value.Symbol[:], symbol)
	value.Seq = seq
	value.NumBidLevels = uint16(len(bids))
	value.NumAskLevels = uint16(len(asks))
	for i, l := range bids {
		value.Bids[i] = PriceLevel{Price: Price{Ticks: l.PriceTicks}, Quantity: Quantity{Ticks: l.SizeTicks}}
	}
	for i, l := range asks {
		value.Asks[i] = PriceLevel{Price: Price{Ticks: l.PriceTicks}, Quantity: Quantity{Ticks: l.SizeTicks}}
	}
	slot.Value = value
	atomic.AddUint32(&slot.Version, 1)
}

// pushDelta writes a delta at the real writer's contract (slot at
// WriteIndex % RingBufferCapacity, then an atomic increment of WriteIndex)
// so tryPop/pollSymbol see it exactly as they would a real engine write.
func pushDelta(queue *BookDeltaRingBuffer, seq uint64, side string, priceTicks, sizeTicks int64) {
	w := atomic.LoadUint64(&queue.WriteIndex)
	var sideByte uint8
	if side == "ask" {
		sideByte = 1
	}
	queue.Slots[w%RingBufferCapacity] = BookDelta{
		Seq: seq, Side: sideByte,
		Price: Price{Ticks: priceTicks}, Size: Quantity{Ticks: sizeTicks},
	}
	atomic.AddUint64(&queue.WriteIndex, 1)
}

func fillOutbox(t *testing.T, c *ClientState) {
	t.Helper()
	for range outboxMaxSize {
		c.Outbox <- "filler"
	}
}

func newLoopbackConnPair(t *testing.T) (server *websocket.Conn, client *websocket.Conn) {
	t.Helper()
	connCh := make(chan *websocket.Conn, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		connCh <- c
	}))
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	clientConn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { clientConn.CloseNow() })

	serverConn := <-connCh
	t.Cleanup(func() { serverConn.CloseNow() })
	return serverConn, clientConn
}

func TestClientStateOverflowDropsTriggerAndReseedsReadySymbol(t *testing.T) {
	poller := newSeededPoller(t, "BTC/USD", 1,
		[]PriceLevelTicks{{PriceTicks: 100, SizeTicks: 5}},
		[]PriceLevelTicks{{PriceTicks: 101, SizeTicks: 5}})

	client := NewClientState(nil)
	client.AddSymbol("BTC/USD")
	client.MarkReady("BTC/USD")

	fillOutbox(t, client)
	client.Enqueue(poller, "TRIGGER")

	if got := len(client.Outbox); got != 1 {
		t.Fatalf("Outbox length after overflow = %d, want 1 (just the reseeded snapshot)", got)
	}
	first := <-client.Outbox
	snap, ok := first.(schemas.BookSnapshotMessage)
	if !ok {
		t.Fatalf("Outbox[0] = %#v (%T), want a BookSnapshotMessage", first, first)
	}
	if snap.Symbol != "BTC/USD" {
		t.Fatalf("reseeded snapshot symbol = %q, want BTC/USD", snap.Symbol)
	}

	// A delta enqueued after the overflow must land strictly after the
	// reseeded snapshot, not race ahead of it.
	client.Enqueue(poller, schemas.NewBookDeltaMessage("BTC/USD", 2, "bid", 100, 4))
	if got := len(client.Outbox); got != 1 {
		t.Fatalf("Outbox length after the follow-up delta = %d, want 1", got)
	}
	second := <-client.Outbox
	delta, ok := second.(schemas.BookDeltaMessage)
	if !ok {
		t.Fatalf("Outbox[1] = %#v (%T), want a BookDeltaMessage", second, second)
	}
	if delta.Seq != 2 {
		t.Fatalf("follow-up delta seq = %d, want 2", delta.Seq)
	}

	if got := poller.Counters().Snapshot().ClientOverflows; got != 1 {
		t.Fatalf("ClientOverflows = %d, want 1", got)
	}
}

func TestClientStatePendingSymbolNeverReseededOnOverflow(t *testing.T) {
	poller := newSeededPoller(t, "BTC/USD", 1,
		[]PriceLevelTicks{{PriceTicks: 100, SizeTicks: 5}},
		[]PriceLevelTicks{{PriceTicks: 101, SizeTicks: 5}})

	client := NewClientState(nil)
	client.AddSymbol("BTC/USD") // never MarkReady -- still pending

	fillOutbox(t, client)
	client.Enqueue(poller, "TRIGGER")

	if got := len(client.Outbox); got != 0 {
		t.Fatalf("Outbox length after overflow with only a pending symbol = %d, want 0 (no reseed)", got)
	}
}

func TestClientStateDisconnectsAfterOverflowLimit(t *testing.T) {
	serverConn, clientConn := newLoopbackConnPair(t)
	poller := newSeededPoller(t, "BTC/USD", 1,
		[]PriceLevelTicks{{PriceTicks: 100, SizeTicks: 5}},
		[]PriceLevelTicks{{PriceTicks: 101, SizeTicks: 5}})

	client := NewClientState(serverConn)
	client.AddSymbol("BTC/USD")
	client.MarkReady("BTC/USD")

	// Read concurrently, before triggering the close: coder/websocket only
	// echoes a close-handshake ack while a Read is in flight, so the
	// server's Conn.Close below would otherwise block for its full
	// handshake timeout waiting for an ack nobody is there to send yet.
	readErrCh := make(chan error, 1)
	go func() {
		_, _, err := clientConn.Read(context.Background())
		readErrCh <- err
	}()

	for range maxOverflowsPerWindow {
		client.collapseAndResync(poller)
	}

	err := <-readErrCh
	if status := websocket.CloseStatus(err); status != websocket.StatusPolicyViolation {
		t.Fatalf("close status = %v (err=%v), want StatusPolicyViolation", status, err)
	}

	if got := poller.Counters().Snapshot().ClientOverflows; got != uint64(maxOverflowsPerWindow) {
		t.Fatalf("ClientOverflows = %d, want %d", got, maxOverflowsPerWindow)
	}
}
