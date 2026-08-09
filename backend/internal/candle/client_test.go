package candle

import "testing"

// The explicit contrast with book's overflow policy: book collapses and
// forces a resnapshot, candle just drops the oldest queued message and
// keeps the newest -- there's no resync concept for candles, so losing an
// old, superseded bar is fine.
func TestClientStateEnqueueDropsOldestKeepsNewestOnOverflow(t *testing.T) {
	c := NewClientState(nil)

	for i := range outboxSize {
		c.Enqueue(i)
	}
	if got := len(c.Outbox); got != outboxSize {
		t.Fatalf("Outbox length after filling to capacity = %d, want %d", got, outboxSize)
	}

	c.Enqueue(outboxSize) // one more than capacity -- must drop the oldest (0)

	if got := len(c.Outbox); got != outboxSize {
		t.Fatalf("Outbox length after overflow = %d, want still %d", got, outboxSize)
	}

	first := <-c.Outbox
	if first != 1 {
		t.Fatalf("oldest surviving message = %v, want 1 (message 0 should have been dropped)", first)
	}

	// Drain the rest and confirm the newest message made it in at the end.
	var last any
	for i := 1; i < outboxSize; i++ {
		last = <-c.Outbox
	}
	if last != outboxSize {
		t.Fatalf("newest message = %v, want %d", last, outboxSize)
	}
}
