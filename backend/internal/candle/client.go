package candle

import "github.com/coder/websocket"

const outboxSize = 64

type ClientState struct {
	Conn   *websocket.Conn
	Outbox chan any
}

func NewClientState(conn *websocket.Conn) *ClientState {
	return &ClientState{Conn: conn, Outbox: make(chan any, outboxSize)}
}

func (c *ClientState) Enqueue(message any) {
	select {
	case c.Outbox <- message:
		return
	default:
	}
	// Full: drop the oldest, then push the new one
	select {
	case <-c.Outbox:
	default:
	}
	select {
	case c.Outbox <- message:
	default:
	}
}
