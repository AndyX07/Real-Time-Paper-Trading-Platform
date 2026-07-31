package trading

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	pb "papertrader/backend/genproto"
	"papertrader/backend/internal/control"
	"papertrader/backend/internal/persistence"
	"papertrader/backend/internal/schemas"

	"google.golang.org/grpc"
)

const (
	// A market order's own immediate fill can reach this watcher before
	// the PlaceOrder call's accept response finishes being persisted --
	// WatchFills is a separate, already-open stream with no ordering
	// relationship to a concurrent unary call's reply (the same class of
	// race the book path already handles via pending/IsPending/MarkReady).
	// Retry briefly rather than treat an unresolved order as permanent.
	fillRecordRetryDelay = 100 * time.Millisecond
	fillRecordRetryLimit = 20 // ~2s total budget

	initialBackoff = 500 * time.Millisecond
	maxBackoff     = 32 * time.Second
)

type FillWatcher struct {
	engineClient *control.EngineClient
	repo         *persistence.Repository
	clients      *ClientRegistry
	stopCh       chan struct{}
	doneCh       chan struct{}
}

func NewFillWatcher(engineClient *control.EngineClient, repo *persistence.Repository, clients *ClientRegistry) *FillWatcher {
	return &FillWatcher{
		engineClient: engineClient, repo: repo, clients: clients,
		stopCh: make(chan struct{}), doneCh: make(chan struct{}),
	}
}

// Stop blocks until Run has actually returned, not just signaled it to --
// main.go closes the persistence DB right after calling this, so Run must
// be guaranteed to have stopped touching repo first, or a fill still being
// processed at shutdown could hit a closed database out from under it.
func (f *FillWatcher) Stop() {
	close(f.stopCh)
	<-f.doneCh
}

func (f *FillWatcher) Run(ctx context.Context) {
	defer close(f.doneCh)
	backoff := initialBackoff

	for {
		select {
		case <-f.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		stream, err := f.engineClient.WatchFills(ctx)
		if err != nil {
			slog.Error("trading.fill_watcher: WatchFills failed to open", "error", err)
			if !f.sleep(ctx, backoff) {
				return
			}
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}

		f.reconcileEngineInstance(ctx)
		backoff = initialBackoff

		f.consume(stream) // blocks until the stream errors or ends
	}
}

func (f *FillWatcher) consume(stream grpc.ServerStreamingClient[pb.FillEvent]) {
	for {
		event, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				slog.Warn("trading.fill_watcher: WatchFills stream ended", "error", err)
			}
			return
		}
		f.handleFill(event)
	}
}

// continuously retrys because fill might come in before order is marked as accepted
func (f *FillWatcher) handleFill(event *pb.FillEvent) {
	var localOrderID int64
	var err error
	for attempt := 0; attempt < fillRecordRetryLimit; attempt++ {
		localOrderID, err = f.repo.RecordFill(event.GetEngineOrderId(), event.GetPriceTicks(), event.GetSizeTicks(),
			int64(event.GetTs()))
		if err == nil {
			break
		}
		if !errors.Is(err, persistence.ErrOrderNotFound) {
			slog.Error("trading.fill_watcher: record fill failed", "engineOrderId", event.GetEngineOrderId(),
				"error", err)
			return
		}
		time.Sleep(fillRecordRetryDelay)
	}
	if err != nil {
		slog.Error("trading.fill_watcher: order never resolved for fill, giving up",
			"engineOrderId", event.GetEngineOrderId())
		return
	}

	f.clients.Broadcast(schemas.NewFillEventMessage(localOrderID, event.GetPriceTicks(), event.GetSizeTicks(),
		event.GetTs()))
	broadcastOrderUpdate(f.repo, f.clients, localOrderID)

	positions, err := f.repo.GetPositions()
	if err != nil {
		slog.Error("trading.fill_watcher: get positions for update broadcast failed", "error", err)
		return
	}
	f.clients.Broadcast(schemas.NewPositionsUpdateMessage(toPositionSnapshots(positions)))
}

func (f *FillWatcher) reconcileEngineInstance(ctx context.Context) {
	infoCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	instanceID, err := f.engineClient.GetEngineInfo(infoCtx)
	cancel()
	if err != nil {
		slog.Error("trading.fill_watcher: get engine info failed, skipping restart reconciliation", "error", err)
		return
	}

	lastID, hadLastID, err := f.repo.GetLastEngineInstanceID()
	if err != nil {
		slog.Error("trading.fill_watcher: get last engine instance id failed", "error", err)
		return
	}

	if !hadLastID || lastID != instanceID {
		orders, err := f.repo.CancelOpenOrdersForEngineRestart()
		if err != nil {
			slog.Error("trading.fill_watcher: cancel open orders for engine restart failed", "error", err)
		} else if len(orders) > 0 {
			slog.Warn("trading.fill_watcher: engine instance changed, canceled stale open orders",
				"count", len(orders))
			for _, order := range orders {
				f.clients.Broadcast(schemas.NewOrderUpdateMessage(toOrderSnapshot(order)))
			}
		}
	}

	if err := f.repo.SetLastEngineInstanceID(instanceID); err != nil {
		slog.Error("trading.fill_watcher: persist engine instance id failed", "error", err)
	}
}

func (f *FillWatcher) sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-f.stopCh:
		return false
	case <-ctx.Done():
		return false
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
