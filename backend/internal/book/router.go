package book

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"papertrader/backend/internal/config"
	"papertrader/backend/internal/control"
	"papertrader/backend/internal/observability"
	"papertrader/backend/internal/schemas"
)

type Router struct {
	poller       *BookPoller
	subs         *SubscriptionManager
	engineClient *control.EngineClient
}

func NewRouter(engineClient *control.EngineClient) *Router {
	r := &Router{
		subs:         NewSubscriptionManager(),
		engineClient: engineClient,
	}
	r.poller = NewBookPoller(r.onDelta, r.onSnapshot)
	return r
}

// NewRouterWithSegment is NewRouter but pre-attaches the poller to an
// in-memory segment instead of the real mmap file, so tests can drive it
// without a running engine.
func NewRouterWithSegment(engineClient *control.EngineClient, segment *SharedMemorySegment) *Router {
	r := NewRouter(engineClient)
	r.poller.segment = segment
	return r
}

func (r *Router) Start(ctx context.Context) {
	go func() {
		if err := r.poller.Run(ctx); err != nil {
			slog.Error("book.router: poller exited", "error", err)
		}
	}()
}

func (r *Router) Stop() {
	r.poller.Stop()
}

func (r *Router) Counters() *observability.BookCounters {
	return r.poller.Counters()
}

func snapshotMessage(event SnapshotEvent) schemas.BookSnapshotMessage {
	bids := make([]schemas.PriceLevelMessage, len(event.Bids))
	for i, l := range event.Bids {
		bids[i] = schemas.PriceLevelMessage{PriceTicks: l.PriceTicks, SizeTicks: l.SizeTicks}
	}
	asks := make([]schemas.PriceLevelMessage, len(event.Asks))
	for i, l := range event.Asks {
		asks[i] = schemas.PriceLevelMessage{PriceTicks: l.PriceTicks, SizeTicks: l.SizeTicks}
	}
	return schemas.NewBookSnapshotMessage(event.Symbol, event.Seq, bids, asks)
}

func deltaMessage(event DeltaEvent) schemas.BookDeltaMessage {
	return schemas.NewBookDeltaMessage(event.Symbol, event.Seq, event.Side, event.PriceTicks, event.SizeTicks)
}

func (r *Router) onDelta(event DeltaEvent) {
	message := deltaMessage(event)
	for _, client := range r.subs.ClientsFor(event.Symbol) {
		if client.IsPending(event.Symbol) {
			continue
		}
		client.Enqueue(r.poller, message)
	}
}

func (r *Router) onSnapshot(event SnapshotEvent) {
	message := snapshotMessage(event)
	for _, client := range r.subs.ClientsFor(event.Symbol) {
		if client.IsPending(event.Symbol) {
			continue
		}
		client.Enqueue(r.poller, message)
	}
}

type incomingMessage struct {
	Type   string `json:"type"`
	Symbol string `json:"symbol"`
}

func (r *Router) HandleWS(w http.ResponseWriter, req *http.Request) {
	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{OriginPatterns: []string{config.DevOrigin}})
	if err != nil {
		slog.Error("book.router: accept failed", "error", err)
		return
	}
	ctx := req.Context()
	client := NewClientState(conn)

	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		r.clientSender(ctx, client)
	}()

	defer func() {
		conn.CloseNow()
		<-senderDone
		emptied := r.subs.UnsubscribeAll(client)
		for _, symbol := range emptied {
			r.poller.UnregisterSymbol(symbol)
			r.engineClient.UnsubscribeBook(context.Background(), symbol)
		}
	}()

	for {
		var msg incomingMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return
		}
		switch msg.Type {
		case "subscribe_book":
			if msg.Symbol != "" {
				r.handleSubscribe(ctx, client, msg.Symbol)
			}
		case "unsubscribe_book":
			if msg.Symbol != "" {
				r.handleUnsubscribe(ctx, client, msg.Symbol)
			}
		}
	}
}

func (r *Router) clientSender(ctx context.Context, client *ClientState) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-client.Outbox:
			if err := wsjson.Write(ctx, client.Conn, msg); err != nil {
				return
			}
		}
	}
}

func (r *Router) handleSubscribe(ctx context.Context, client *ClientState, symbol string) {
	isNew := r.subs.Subscribe(symbol, client)
	client.AddSymbol(symbol)

	var snapshot *SnapshotEvent
	if isNew {
		result := r.engineClient.SubscribeBook(ctx, symbol)
		if !result.Ok {
			slog.Warn("book.router: engine rejected subscribe", "symbol", symbol, "reason", result.Reason)
			r.subs.Unsubscribe(symbol, client)
			client.RemoveSymbol(symbol)
			return
		}
		registerCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		defer cancel()
		s, err := r.poller.RegisterSymbol(registerCtx, symbol)
		if err != nil {
			slog.Error("book.router: register_symbol failed", "symbol", symbol, "error", err)
			return
		}
		snapshot = &s
	} else {
		snapshot = r.poller.CurrentSnapshot(symbol)
	}
	if snapshot != nil {
		client.Enqueue(r.poller, snapshotMessage(*snapshot))
	}
	client.MarkReady(symbol)
}

func (r *Router) handleUnsubscribe(ctx context.Context, client *ClientState, symbol string) {
	client.RemoveSymbol(symbol)
	if r.subs.Unsubscribe(symbol, client) {
		r.poller.UnregisterSymbol(symbol)
		r.engineClient.UnsubscribeBook(ctx, symbol)
	}
}
