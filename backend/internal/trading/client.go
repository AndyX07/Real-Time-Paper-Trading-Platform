package trading

import (
	"fmt"
	"log/slog"

	"github.com/coder/websocket"
)

const outboxMaxSize = 100

type ClientState struct {
	Conn   *websocket.Conn
	Outbox chan any
}

func NewClientState(conn *websocket.Conn) *ClientState {
	return &ClientState{Conn: conn, Outbox: make(chan any, outboxMaxSize)}
}

func (c *ClientState) Enqueue(message any) {
	select {
	case c.Outbox <- message:
	default:
		slog.Warn("trading.client: outbox full, dropping message", "type", fmt.Sprintf("%T", message))
	}
}
