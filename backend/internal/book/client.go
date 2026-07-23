package book

import (
	"sync"

	"github.com/coder/websocket"
)

const outboxMaxSize = 500
const maxConsecutiveOverflows = 5

type ClientState struct {
	Conn   *websocket.Conn
	Outbox chan any

	mu                   sync.Mutex
	Symbols              map[string]struct{}
	pending              map[string]struct{} // subscribed but no seed data
	ConsecutiveOverflows int
}

func NewClientState(conn *websocket.Conn) *ClientState {
	return &ClientState{
		Conn:    conn,
		Outbox:  make(chan any, outboxMaxSize),
		Symbols: make(map[string]struct{}),
		pending: make(map[string]struct{}),
	}
}

func (c *ClientState) AddSymbol(symbol string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Symbols[symbol] = struct{}{}
	c.pending[symbol] = struct{}{}
}

func (c *ClientState) MarkReady(symbol string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, symbol)
}

func (c *ClientState) IsPending(symbol string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, pending := c.pending[symbol]
	return pending
}

func (c *ClientState) RemoveSymbol(symbol string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Symbols, symbol)
	delete(c.pending, symbol)
}

func (c *ClientState) SymbolSnapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	symbols := make([]string, 0, len(c.Symbols))
	for s := range c.Symbols {
		symbols = append(symbols, s)
	}
	return symbols
}

func (c *ClientState) Enqueue(poller *BookPoller, message any) {
	select {
	case c.Outbox <- message:
		c.mu.Lock()
		c.ConsecutiveOverflows = 0
		c.mu.Unlock()
	default:
		c.collapseAndResync(poller)
	}
}

func (c *ClientState) collapseAndResync(poller *BookPoller) {
	c.mu.Lock()
	c.ConsecutiveOverflows++
	overflows := c.ConsecutiveOverflows
	c.mu.Unlock()

drain:
	for {
		select {
		case <-c.Outbox:
		default:
			break drain
		}
	}

	for _, symbol := range c.SymbolSnapshot() {
		if c.IsPending(symbol) {
			continue
		}
		if snapshot := poller.CurrentSnapshot(symbol); snapshot != nil {
			select {
			case c.Outbox <- snapshotMessage(*snapshot):
			default:
			}
		}
	}

	if overflows >= maxConsecutiveOverflows {
		c.Conn.Close(websocket.StatusPolicyViolation, "stuck past overflow limit")
	}
}
