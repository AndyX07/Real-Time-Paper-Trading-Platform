package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"google.golang.org/grpc"

	pb "papertrader/backend/genproto"
	"papertrader/backend/internal/book"
	"papertrader/backend/internal/candle"
	"papertrader/backend/internal/control"
)

// --- fake engine control-plane (gRPC) ---

type fakeEngineServer struct {
	pb.UnimplementedEngineControlServer
}

func (fakeEngineServer) SubscribeBook(context.Context, *pb.SubscribeRequest) (*pb.SubscribeReply, error) {
	return &pb.SubscribeReply{Ok: true}, nil
}

func (fakeEngineServer) UnsubscribeBook(context.Context, *pb.SubscribeRequest) (*pb.SubscribeReply, error) {
	return &pb.SubscribeReply{Ok: true}, nil
}

func startFakeEngine(t *testing.T) *control.EngineClient {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterEngineControlServer(grpcServer, fakeEngineServer{})
	go grpcServer.Serve(lis)
	t.Cleanup(grpcServer.Stop)

	port := strconv.Itoa(lis.Addr().(*net.TCPAddr).Port)
	t.Setenv("ENGINE_GRPC_PORT", port)

	engineClient := control.New()
	if err := engineClient.Start(); err != nil {
		t.Fatalf("engine client start: %v", err)
	}
	t.Cleanup(func() { engineClient.Close() })
	return engineClient
}

// --- in-memory shared-memory segment, written directly via book's exported
// layout types (mirroring the real engine writer's contract) ---

func writeSnapshot(slot *book.SnapshotSlot, symbol string, seq uint64, priceTicks, sizeTicks int64) {
	atomic.AddUint32(&slot.Version, 1)
	var value book.BookSnapshot
	copy(value.Symbol[:], symbol)
	value.Seq = seq
	value.NumBidLevels = 1
	value.Bids[0] = book.PriceLevel{Price: book.Price{Ticks: priceTicks}, Quantity: book.Quantity{Ticks: sizeTicks}}
	slot.Value = value
	atomic.AddUint32(&slot.Version, 1)
}

func pushDelta(queue *book.BookDeltaRingBuffer, seq uint64, priceTicks, sizeTicks int64) {
	w := atomic.LoadUint64(&queue.WriteIndex)
	queue.Slots[w%book.RingBufferCapacity] = book.BookDelta{
		Seq: seq, Side: 0, Price: book.Price{Ticks: priceTicks}, Size: book.Quantity{Ticks: sizeTicks},
	}
	atomic.AddUint64(&queue.WriteIndex, 1)
}

// onceStop wraps a done-channel close so the returned func is safe to call
// more than once (e.g. once explicitly mid-test, once via t.Cleanup).
func onceStop(done chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

// startBookDeltaDriver continuously pushes new deltas for symbol into the
// segment's ring buffer until the returned func is called.
func startBookDeltaDriver(segment *book.SharedMemorySegment, startSeq uint64) (stop func()) {
	done := make(chan struct{})
	go func() {
		seq := startSeq
		for {
			select {
			case <-done:
				return
			default:
			}
			pushDelta(&segment.Slots[0].DeltaQueue, seq, 100, int64(seq))
			seq++
			time.Sleep(5 * time.Millisecond)
		}
	}()
	return onceStop(done)
}

// --- fake Kraken WS server for the candle side ---

type mockKraken struct {
	ts     *httptest.Server
	connCh chan *websocket.Conn
}

func startMockKraken(t *testing.T) *mockKraken {
	t.Helper()
	m := &mockKraken{connCh: make(chan *websocket.Conn, 4)}
	m.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		m.connCh <- c
	}))
	t.Cleanup(m.ts.Close)
	return m
}

func (m *mockKraken) wsURL() string {
	return "ws" + strings.TrimPrefix(m.ts.URL, "http")
}

// startCandleDriver waits for a connection, drains its subscribe frame,
// then continuously pushes candle frames on it until the returned func is
// called.
func startCandleDriver(t *testing.T, m *mockKraken) (stop func()) {
	t.Helper()
	done := make(chan struct{})
	connCh := make(chan *websocket.Conn, 1)
	go func() {
		var conn *websocket.Conn
		select {
		case conn = <-m.connCh:
		case <-done:
			return
		}
		connCh <- conn
		conn.Read(context.Background()) // drain the subscribe frame

		second := 0
		for {
			select {
			case <-done:
				return
			default:
			}
			second = (second + 1) % 60
			msg := fmt.Sprintf(`{"channel":"ohlc","data":[{"symbol":"BTC/USD","interval":1,`+
				`"interval_begin":"2024-01-01T00:00:%02d.000000000Z","open":100,"high":101,"low":99,"close":100.5,"volume":1}]}`,
				second)
			if err := conn.Write(context.Background(), websocket.MessageText, []byte(msg)); err != nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Closing done alone isn't enough: the accepted server-side conn stays
	// open from httptest's point of view (it was hijacked, not returned to
	// the server), so Server.Close would otherwise block on it. Close the
	// conn too, once the goroutine has had a chance to publish it.
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			select {
			case conn := <-connCh:
				conn.Close(websocket.StatusNormalClosure, "test driver stopped")
			case <-time.After(200 * time.Millisecond):
			}
		})
	}
}

// --- real WS client helpers driving the mux exactly as a browser would ---

func dialWS(t *testing.T, serverURL, path string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + path
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", path, err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func readMessagesInto(conn *websocket.Conn, ch chan<- map[string]any) {
	for {
		_, data, err := conn.Read(context.Background())
		if err != nil {
			return
		}
		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		select {
		case ch <- msg:
		default:
		}
	}
}

func dialAndSubscribeBook(t *testing.T, serverURL, symbol string) <-chan map[string]any {
	t.Helper()
	conn := dialWS(t, serverURL, "/ws/book")
	if err := wsjson.Write(context.Background(), conn, map[string]any{"type": "subscribe_book", "symbol": symbol}); err != nil {
		t.Fatalf("subscribe_book: %v", err)
	}
	ch := make(chan map[string]any, 64)
	go readMessagesInto(conn, ch)
	return ch
}

func dialAndSubscribeCandle(t *testing.T, serverURL, symbol string, interval int) <-chan map[string]any {
	t.Helper()
	conn := dialWS(t, serverURL, "/ws/candles")
	if err := wsjson.Write(context.Background(), conn,
		map[string]any{"type": "subscribe_candle", "symbol": symbol, "interval": interval}); err != nil {
		t.Fatalf("subscribe_candle: %v", err)
	}
	ch := make(chan map[string]any, 64)
	go readMessagesInto(conn, ch)
	return ch
}

func countMessages(ch <-chan map[string]any, d time.Duration) int {
	deadline := time.After(d)
	n := 0
	for {
		select {
		case <-ch:
			n++
		case <-deadline:
			return n
		}
	}
}

// --- test environment ---

type testEnv struct {
	server       *httptest.Server
	segment      *book.SharedMemorySegment
	kraken       *mockKraken
	bookRouter   *book.Router
	candleRouter *candle.Router
	stopBook     func() // idempotent: Router.Stop itself is not safe to call twice
	stopBookGen  func()
	stopCandle   func()
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	engineClient := startFakeEngine(t)

	segment := &book.SharedMemorySegment{}
	slot := &segment.Slots[0]
	slot.Claimed = 1
	copy(slot.Symbol[:], "BTC/USD")
	writeSnapshot(&slot.Snapshot, "BTC/USD", 1, 100, 5)

	kraken := startMockKraken(t)

	bookRouter := book.NewRouterWithSegment(engineClient, segment)
	candleRouter := candle.NewRouterWithWSURL(kraken.wsURL())

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	bookRouter.Start(ctx)
	var stopBookOnce sync.Once
	stopBook := func() { stopBookOnce.Do(bookRouter.Stop) } // Router.Stop itself isn't safe to call twice
	t.Cleanup(stopBook)
	candleRouter.Start(ctx)
	t.Cleanup(candleRouter.Stop)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/book", bookRouter.HandleWS)
	mux.HandleFunc("/ws/candles", candleRouter.HandleWS)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	stopBookGen := startBookDeltaDriver(segment, 2)
	t.Cleanup(stopBookGen)
	stopCandle := startCandleDriver(t, kraken)
	t.Cleanup(stopCandle)

	return &testEnv{
		server: server, segment: segment, kraken: kraken,
		bookRouter: bookRouter, candleRouter: candleRouter,
		stopBook: stopBook, stopBookGen: stopBookGen, stopCandle: stopCandle,
	}
}

func TestPathIndependenceCandleFailureDoesNotAffectBook(t *testing.T) {
	env := newTestEnv(t)

	bookCh := dialAndSubscribeBook(t, env.server.URL, "BTC/USD")
	candleCh := dialAndSubscribeCandle(t, env.server.URL, "BTC/USD", 1)

	if n := countMessages(bookCh, 500*time.Millisecond); n == 0 {
		t.Fatal("expected book deltas before killing the candle feed")
	}
	if n := countMessages(candleCh, 500*time.Millisecond); n == 0 {
		t.Fatal("expected candle messages before the kill")
	}

	// Kill the candle feed: stop the driver and take down the mock Kraken
	// server entirely so the client's automatic reconnect can never
	// succeed again for the rest of this test.
	env.stopCandle()
	env.kraken.ts.CloseClientConnections()
	env.kraken.ts.Close()

	if n := countMessages(bookCh, 500*time.Millisecond); n == 0 {
		t.Fatal("book deltas stopped after killing the unrelated candle feed -- failure domains are not independent")
	}
}

func TestPathIndependenceBookFailureDoesNotAffectCandle(t *testing.T) {
	env := newTestEnv(t)

	bookCh := dialAndSubscribeBook(t, env.server.URL, "BTC/USD")
	candleCh := dialAndSubscribeCandle(t, env.server.URL, "BTC/USD", 1)

	if n := countMessages(bookCh, 500*time.Millisecond); n == 0 {
		t.Fatal("expected book deltas before killing the book feed")
	}
	if n := countMessages(candleCh, 500*time.Millisecond); n == 0 {
		t.Fatal("expected candle messages before the kill")
	}

	env.stopBook()
	env.stopBookGen()

	if n := countMessages(candleCh, 500*time.Millisecond); n == 0 {
		t.Fatal("candle messages stopped after killing the unrelated book feed -- failure domains are not independent")
	}
}
