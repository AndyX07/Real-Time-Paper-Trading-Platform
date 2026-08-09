package candle

import (
	"testing"

	"papertrader/backend/internal/schemas"
)

func TestFormatInterval(t *testing.T) {
	cases := []struct {
		minutes int
		want    string
	}{
		{1, "1m"},
		{30, "30m"},
		{60, "1h"},
		{120, "2h"},
		{1439, "23h"},
		{1440, "1d"},
		{2880, "2d"},
	}
	for _, tc := range cases {
		if got := formatInterval(tc.minutes); got != tc.want {
			t.Errorf("formatInterval(%d) = %q, want %q", tc.minutes, got, tc.want)
		}
	}
}

func TestToUnixSeconds(t *testing.T) {
	if got := toUnixSeconds(""); got != 0 {
		t.Errorf("toUnixSeconds(\"\") = %d, want 0", got)
	}
	if got := toUnixSeconds("not-a-timestamp"); got != 0 {
		t.Errorf("toUnixSeconds(garbage) = %d, want 0", got)
	}
	// 2024-01-01T00:00:00Z == 1704067200
	if got := toUnixSeconds("2024-01-01T00:00:00.000000000Z"); got != 1704067200 {
		t.Errorf("toUnixSeconds(2024-01-01T00:00:00Z) = %d, want 1704067200", got)
	}
}

func newTestOhlcClient(t *testing.T) (*OhlcClient, *[]schemas.CandleMessage) {
	t.Helper()
	var received []schemas.CandleMessage
	c := NewOhlcClient(func(symbol string, intervalMinutes int, candle schemas.CandleMessage) {
		received = append(received, candle)
	})
	return c, &received
}

func TestHandleCandleEntryFirstEntryEmitsOpen(t *testing.T) {
	c, received := newTestOhlcClient(t)

	c.handleCandleEntry(ohlcEntry{
		Symbol: "BTC/USD", Interval: 1, IntervalBegin: "2024-01-01T00:00:00.000000000Z",
		Open: 100, High: 101, Low: 99, Close: 100.5, Volume: 10,
	})

	if len(*received) != 1 {
		t.Fatalf("received = %v, want exactly one candle", *received)
	}
	got := (*received)[0]
	if got.Closed {
		t.Fatalf("first entry's candle should not be closed yet, got %+v", got)
	}
	if got.Open != 100 || got.Close != 100.5 {
		t.Fatalf("candle fields = %+v, want Open=100 Close=100.5", got)
	}
}

func TestHandleCandleEntrySameIntervalUpdatesInPlace(t *testing.T) {
	c, received := newTestOhlcClient(t)

	c.handleCandleEntry(ohlcEntry{
		Symbol: "BTC/USD", Interval: 1, IntervalBegin: "2024-01-01T00:00:00.000000000Z",
		Open: 100, High: 101, Low: 99, Close: 100.5, Volume: 10,
	})
	c.handleCandleEntry(ohlcEntry{
		Symbol: "BTC/USD", Interval: 1, IntervalBegin: "2024-01-01T00:00:00.000000000Z",
		Open: 100, High: 102, Low: 99, Close: 101.5, Volume: 15,
	})

	if len(*received) != 2 {
		t.Fatalf("received = %v, want two dispatches (both updates to the same open bar)", *received)
	}
	for i, got := range *received {
		if got.Closed {
			t.Fatalf("dispatch %d should not be closed (same interval still open): %+v", i, got)
		}
	}
	if last := (*received)[1]; last.Close != 101.5 || last.High != 102 {
		t.Fatalf("second dispatch = %+v, want the updated High=102 Close=101.5", last)
	}
}

func TestHandleCandleEntryNewIntervalClosesPreviousThenOpensNext(t *testing.T) {
	c, received := newTestOhlcClient(t)

	c.handleCandleEntry(ohlcEntry{
		Symbol: "BTC/USD", Interval: 1, IntervalBegin: "2024-01-01T00:00:00.000000000Z",
		Open: 100, High: 101, Low: 99, Close: 100.5, Volume: 10,
	})
	c.handleCandleEntry(ohlcEntry{
		Symbol: "BTC/USD", Interval: 1, IntervalBegin: "2024-01-01T00:01:00.000000000Z",
		Open: 100.5, High: 103, Low: 100, Close: 102, Volume: 5,
	})

	if len(*received) != 3 {
		t.Fatalf("received = %v, want 3 dispatches: open, close-of-previous, open-of-next", *received)
	}
	if (*received)[0].Closed {
		t.Fatalf("dispatch 0 (first bar's open) should not be closed: %+v", (*received)[0])
	}
	closedPrev := (*received)[1]
	if !closedPrev.Closed {
		t.Fatalf("dispatch 1 must be the previous bar marked closed, got %+v", closedPrev)
	}
	if closedPrev.Close != 100.5 {
		t.Fatalf("closed dispatch should still carry the first bar's data (Close=100.5), got %+v", closedPrev)
	}
	next := (*received)[2]
	if next.Closed {
		t.Fatalf("dispatch 2 (the newly opened bar) should not be closed: %+v", next)
	}
	if next.Close != 102 {
		t.Fatalf("newly opened bar = %+v, want Close=102", next)
	}
}

func TestHandleMessageMalformedJSONDoesNotPanic(t *testing.T) {
	c, received := newTestOhlcClient(t)

	c.handleMessage([]byte("not json at all"))
	c.handleMessage([]byte(`{"channel":"ohlc","data":`)) // truncated
	c.handleMessage([]byte(`{"channel":"heartbeat"}`))   // valid JSON, wrong channel

	if len(*received) != 0 {
		t.Fatalf("received = %v, want none of these to produce a candle", *received)
	}
}

func TestHandleMessageValidEnvelopeDispatches(t *testing.T) {
	c, received := newTestOhlcClient(t)

	c.handleMessage([]byte(`{"channel":"ohlc","data":[{"symbol":"BTC/USD","interval":1,
		"interval_begin":"2024-01-01T00:00:00.000000000Z","open":100,"high":101,"low":99,"close":100.5,"volume":10}]}`))

	if len(*received) != 1 {
		t.Fatalf("received = %v, want exactly one candle from a valid envelope", *received)
	}
}
