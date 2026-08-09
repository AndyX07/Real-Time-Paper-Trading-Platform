// TestEngineCrossProcessRealSharedMemoryAgreement spawns the real C++
// test_engine_harness binary (see engine/tools/replay/test_engine_harness.cpp)
// and drives it with the real book.NewRouter/control.EngineClient
// production code, so the actual cross-language mmap boundary gets
// exercised end to end -- not just simulated in Go on both sides, as
// TestPathIndependence* does.
//
// Opt-in only: set PAPER_TRADER_TEST_ENGINE_BIN to a built harness binary
// path, e.g.
//
//	PAPER_TRADER_TEST_ENGINE_BIN=engine/out/build/<preset>/tools/replay/test_engine_harness.exe \
//	    go test -run CrossProcess -v ./internal/integration/...
//
// The test is otherwise network-free and deterministic: the harness
// replays the committed golden_session.ptrec fixture instead of connecting
// to live Kraken, and uses a fixed, test-only segment name/port that can
// never collide with a developer's real running dev engine.
package integration

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"papertrader/backend/internal/book"
	"papertrader/backend/internal/control"
)

const (
	crossProcessSegmentName = "paper_trader_test_fixed"
	crossProcessGRPCPort    = "57050"
	crossProcessGRPCAddr    = "127.0.0.1:" + crossProcessGRPCPort
)

func waitForTCPReady(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to accept connections", addr)
}

// countMessageTypesUntil counts messages by their "type" field, returning
// as soon as every type in want has been seen at least once (or d elapses,
// whichever comes first) -- keeps this network-free test from always
// paying the full timeout on the happy path.
func countMessageTypesUntil(ch <-chan map[string]any, d time.Duration, want ...string) map[string]int {
	deadline := time.After(d)
	counts := make(map[string]int)
	seen := func() bool {
		for _, w := range want {
			if counts[w] == 0 {
				return false
			}
		}
		return true
	}
	for {
		select {
		case msg := <-ch:
			if t, ok := msg["type"].(string); ok {
				counts[t]++
			}
			if seen() {
				return counts
			}
		case <-deadline:
			return counts
		}
	}
}

func TestEngineCrossProcessRealSharedMemoryAgreement(t *testing.T) {
	binPath := os.Getenv("PAPER_TRADER_TEST_ENGINE_BIN")
	if binPath == "" {
		t.Skip("PAPER_TRADER_TEST_ENGINE_BIN not set -- skipping cross-process engine<->backend test (see file header for how to run it)")
	}

	var output bytes.Buffer
	cmd := exec.Command(binPath)
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start test engine harness at %s: %v", binPath, err)
	}

	// Registered first so it runs last (t.Cleanup is LIFO): only log once
	// the kill+wait cleanup below has finished populating the buffer.
	t.Cleanup(func() { t.Logf("test_engine_harness output:\n%s", output.String()) })
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	waitForTCPReady(t, crossProcessGRPCAddr, 10*time.Second)

	origOverride := book.SegmentNameOverride
	book.SegmentNameOverride = crossProcessSegmentName
	t.Cleanup(func() { book.SegmentNameOverride = origOverride })

	t.Setenv("ENGINE_GRPC_PORT", crossProcessGRPCPort)
	engineClient := control.New()
	if err := engineClient.Start(); err != nil {
		t.Fatalf("engine client start: %v", err)
	}
	t.Cleanup(func() { engineClient.Close() })

	// The real production constructor and mmap-attach path -- not
	// NewRouterWithSegment's in-memory seeding shortcut.
	bookRouter := book.NewRouter(engineClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	bookRouter.Start(ctx)
	t.Cleanup(bookRouter.Stop)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/book", bookRouter.HandleWS)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	bookCh := dialAndSubscribeBook(t, server.URL, "BTC/USD")

	counts := countMessageTypesUntil(bookCh, 10*time.Second, "book_snapshot", "book_delta")
	if counts["book_snapshot"] == 0 {
		t.Fatalf("no book_snapshot received from the real engine harness (message counts: %v)", counts)
	}
	if counts["book_delta"] == 0 {
		t.Fatalf("no book_delta received from the real engine harness (message counts: %v)", counts)
	}
}
