package trading

import "sync"

type ClientRegistry struct {
	mu      sync.Mutex
	clients map[*ClientState]struct{}
}

func NewClientRegistry() *ClientRegistry {
	return &ClientRegistry{clients: make(map[*ClientState]struct{})}
}

func (r *ClientRegistry) Add(client *ClientState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[client] = struct{}{}
}

func (r *ClientRegistry) Remove(client *ClientState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, client)
}

func (r *ClientRegistry) Broadcast(message any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for client := range r.clients {
		client.Enqueue(message)
	}
}
