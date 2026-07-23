package candle

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"papertrader/backend/internal/config"
	"papertrader/backend/internal/schemas"
)

type Router struct {
	subs   *SubscriptionManager
	client *OhlcClient
}

func NewRouter() *Router {
	r := &Router{subs: NewSubscriptionManager()}
	r.client = NewOhlcClient(r.onCandle)
	return r
}

func (r *Router) Start(ctx context.Context) {
	go r.client.Run(ctx)
}

func (r *Router) Stop() {
	r.client.Stop()
}

func (r *Router) onCandle(symbol string, intervalMinutes int, candle schemas.CandleMessage) {
	for _, client := range r.subs.ClientsFor(symbol, intervalMinutes) {
		client.Enqueue(candle)
	}
}

func (r *Router) HandleHistory(w http.ResponseWriter, req *http.Request) {
	symbol := req.URL.Query().Get("symbol")
	intervalMinutes := 1
	if v := req.URL.Query().Get("interval"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			intervalMinutes = parsed
		}
	}
	count := 720
	if v := req.URL.Query().Get("count"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed <= 720 {
			count = parsed
		}
	}

	bars, err := FetchOhlcHistory(req.Context(), symbol, intervalMinutes, count)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		slog.Error("candle.router: backfill fetch failed", "symbol", symbol, "interval", intervalMinutes, "error", err)
		json.NewEncoder(w).Encode([]HistoryBar{})
		return
	}
	json.NewEncoder(w).Encode(bars)
}

type incomingMessage struct {
	Type     string `json:"type"`
	Symbol   string `json:"symbol"`
	Interval int    `json:"interval"`
}

func (r *Router) HandleWS(w http.ResponseWriter, req *http.Request) {
	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{OriginPatterns: []string{config.DevOrigin}})
	if err != nil {
		slog.Error("candle.router: accept failed", "error", err)
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
		for _, key := range emptied {
			r.client.Unsubscribe(context.Background(), key.Symbol, key.IntervalMinutes)
		}
	}()

	for {
		var msg incomingMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return
		}
		intervalMinutes := msg.Interval
		if intervalMinutes == 0 {
			intervalMinutes = 1
		}
		switch msg.Type {
		case "subscribe_candle":
			if msg.Symbol != "" {
				if r.subs.Subscribe(msg.Symbol, intervalMinutes, client) {
					r.client.Subscribe(ctx, msg.Symbol, intervalMinutes)
				}
			}
		case "unsubscribe_candle":
			if msg.Symbol != "" {
				if r.subs.Unsubscribe(msg.Symbol, intervalMinutes, client) {
					r.client.Unsubscribe(ctx, msg.Symbol, intervalMinutes)
				}
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
