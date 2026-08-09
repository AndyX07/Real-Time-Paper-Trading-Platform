package candle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"papertrader/backend/internal/schemas"
)

type mockKrakenServer struct {
	connCh chan *websocket.Conn
}

func newMockKrakenServer(t *testing.T) (*httptest.Server, *mockKrakenServer) {
	t.Helper()
	m := &mockKrakenServer{connCh: make(chan *websocket.Conn, 4)}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		m.connCh <- c
	}))
	t.Cleanup(ts.Close)
	return ts, m
}

func assertSubscribeFrame(t *testing.T, conn *websocket.Conn, wantSymbol string, wantInterval int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read subscribe frame: %v", err)
	}
	var msg subscribeMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal subscribe frame: %v", err)
	}
	if msg.Method != "subscribe" {
		t.Fatalf("method = %q, want subscribe", msg.Method)
	}
	if msg.Params.Channel != "ohlc" || len(msg.Params.Symbol) != 1 || msg.Params.Symbol[0] != wantSymbol ||
		msg.Params.Interval != wantInterval {
		t.Fatalf("params = %+v, want symbol=%s interval=%d", msg.Params, wantSymbol, wantInterval)
	}
}

// TestOhlcClientReconnectsAndResubscribesTransparently proves both halves
// of the reconnect story end to end: a dropped connection is retried, and
// the durable c.subscribed map is replayed onto the new connection without
// the caller having to re-subscribe.
func TestOhlcClientReconnectsAndResubscribesTransparently(t *testing.T) {
	origBackoff := InitialBackoff
	InitialBackoff = 10 * time.Millisecond
	t.Cleanup(func() { InitialBackoff = origBackoff })

	ts, mock := newMockKrakenServer(t)

	var mu sync.Mutex
	var candles []schemas.CandleMessage
	client := NewOhlcClient(func(symbol string, interval int, c schemas.CandleMessage) {
		mu.Lock()
		candles = append(candles, c)
		mu.Unlock()
	})
	client.wsURL = "ws" + strings.TrimPrefix(ts.URL, "http")

	// Subscribed before Run even starts -- no connection exists yet, so
	// this only records intent in c.subscribed.
	client.Subscribe(context.Background(), "BTC/USD", 1)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go client.Run(ctx)

	conn1 := <-mock.connCh
	assertSubscribeFrame(t, conn1, "BTC/USD", 1)
	conn1.Close(websocket.StatusNormalClosure, "simulated drop")

	conn2 := <-mock.connCh
	assertSubscribeFrame(t, conn2, "BTC/USD", 1) // resubscribed on the new connection, without the test calling Subscribe again

	candleJSON := `{"channel":"ohlc","data":[{"symbol":"BTC/USD","interval":1,
		"interval_begin":"2024-01-01T00:00:00.000000000Z","open":100,"high":101,"low":99,"close":100.5,"volume":10}]}`
	if err := conn2.Write(context.Background(), websocket.MessageText, []byte(candleJSON)); err != nil {
		t.Fatalf("write candle frame: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(candles)
		mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for a candle delivered over the reconnected connection")
}
