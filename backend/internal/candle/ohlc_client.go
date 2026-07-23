package candle

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"

	"papertrader/backend/internal/config"
	"papertrader/backend/internal/schemas"
)

type CandleCallback func(symbol string, intervalMinutes int, candle schemas.CandleMessage)

func formatInterval(minutes int) string {
	switch {
	case minutes < 60:
		return fmt.Sprintf("%dm", minutes)
	case minutes < 1440:
		return fmt.Sprintf("%dh", minutes/60)
	default:
		return fmt.Sprintf("%dd", minutes/1440)
	}
}

func toUnixSeconds(iso string) int64 {
	if iso == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		return 0
	}
	return t.UTC().Unix()
}

type OhlcClient struct {
	onCandle CandleCallback

	mu                sync.Mutex
	subscribed        map[SubKey]struct{}
	lastIntervalBegin map[SubKey]string
	lastCandle        map[SubKey]schemas.CandleMessage
	conn              *websocket.Conn
	stopped           bool
}

func NewOhlcClient(onCandle CandleCallback) *OhlcClient {
	return &OhlcClient{
		onCandle:          onCandle,
		subscribed:        make(map[SubKey]struct{}),
		lastIntervalBegin: make(map[SubKey]string),
		lastCandle:        make(map[SubKey]schemas.CandleMessage),
	}
}

func (c *OhlcClient) Run(ctx context.Context) {
	backoff := 1 * time.Second
	for {
		err := c.runOnce(ctx)

		c.mu.Lock()
		stopped := c.stopped
		c.mu.Unlock()
		if stopped || ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Error("candle.kraken connection error, reconnecting", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 64*time.Second {
			backoff = 64 * time.Second
		}
	}
}

func (c *OhlcClient) Stop() {
	c.mu.Lock()
	c.stopped = true
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		conn.Close(websocket.StatusNormalClosure, "shutting down")
	}
}

func (c *OhlcClient) runOnce(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, config.KrakenOhlcWSURL, nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	c.mu.Lock()
	c.conn = conn
	keys := make([]SubKey, 0, len(c.subscribed))
	for k := range c.subscribed {
		keys = append(keys, k)
	}
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
	}()

	slog.Info("candle.kraken connected")

	for _, k := range keys {
		if err := sendSubscribe(ctx, conn, k.Symbol, k.IntervalMinutes); err != nil {
			return err
		}
	}

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		c.handleMessage(data)
	}
}

func (c *OhlcClient) Subscribe(ctx context.Context, symbol string, intervalMinutes int) {
	key := SubKey{symbol, intervalMinutes}
	c.mu.Lock()
	c.subscribed[key] = struct{}{}
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		_ = sendSubscribe(ctx, conn, symbol, intervalMinutes)
	}
}

func (c *OhlcClient) Unsubscribe(ctx context.Context, symbol string, intervalMinutes int) {
	key := SubKey{symbol, intervalMinutes}
	c.mu.Lock()
	delete(c.subscribed, key)
	delete(c.lastIntervalBegin, key)
	delete(c.lastCandle, key)
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		_ = sendUnsubscribe(ctx, conn, symbol, intervalMinutes)
	}
}

type subscribeParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Interval int      `json:"interval"`
}

type subscribeMessage struct {
	Method string          `json:"method"`
	Params subscribeParams `json:"params"`
}

func sendSubscribe(ctx context.Context, conn *websocket.Conn, symbol string, intervalMinutes int) error {
	msg := subscribeMessage{Method: "subscribe", Params: subscribeParams{Channel: "ohlc", Symbol: []string{symbol}, Interval: intervalMinutes}}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return err
	}
	slog.Info("candle.kraken subscribed", "symbol", symbol, "interval", intervalMinutes)
	return nil
}

func sendUnsubscribe(ctx context.Context, conn *websocket.Conn, symbol string, intervalMinutes int) error {
	msg := subscribeMessage{Method: "unsubscribe", Params: subscribeParams{Channel: "ohlc", Symbol: []string{symbol}, Interval: intervalMinutes}}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return err
	}
	slog.Info("candle.kraken unsubscribed", "symbol", symbol, "interval", intervalMinutes)
	return nil
}

type ohlcEntry struct {
	Symbol        string  `json:"symbol"`
	Interval      int     `json:"interval"`
	IntervalBegin string  `json:"interval_begin"`
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Close         float64 `json:"close"`
	Volume        float64 `json:"volume"`
}

type ohlcEnvelope struct {
	Channel string      `json:"channel"`
	Data    []ohlcEntry `json:"data"`
}

func (c *OhlcClient) handleMessage(data []byte) {
	var envelope ohlcEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return
	}
	if envelope.Channel != "ohlc" {
		return
	}
	for _, entry := range envelope.Data {
		c.handleCandleEntry(entry)
	}
}

func (c *OhlcClient) handleCandleEntry(entry ohlcEntry) {
	if entry.Symbol == "" {
		return
	}
	key := SubKey{entry.Symbol, entry.Interval}

	candle := schemas.CandleMessage{
		Type:     "candle",
		Symbol:   entry.Symbol,
		Interval: formatInterval(entry.Interval),
		Time:     toUnixSeconds(entry.IntervalBegin),
		Open:     entry.Open,
		High:     entry.High,
		Low:      entry.Low,
		Close:    entry.Close,
		Volume:   entry.Volume,
		Closed:   false,
	}

	c.mu.Lock()
	prevBegin, hadPrev := c.lastIntervalBegin[key]
	prevCandle, hadPrevCandle := c.lastCandle[key]
	closeOutPrev := hadPrev && entry.IntervalBegin != prevBegin && hadPrevCandle
	c.lastIntervalBegin[key] = entry.IntervalBegin
	c.lastCandle[key] = candle
	c.mu.Unlock()

	if closeOutPrev {
		prevCandle.Closed = true
		c.onCandle(entry.Symbol, entry.Interval, prevCandle)
	}
	c.onCandle(entry.Symbol, entry.Interval, candle)
}
