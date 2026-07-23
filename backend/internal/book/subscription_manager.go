package book

import "sync"

type SubscriptionManager struct {
	mu              sync.Mutex
	clientsBySymbol map[string]map[*ClientState]struct{}
}

func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{clientsBySymbol: make(map[string]map[*ClientState]struct{})}
}

// true if it's the first subscriber
func (m *SubscriptionManager) Subscribe(symbol string, client *ClientState) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	clients, ok := m.clientsBySymbol[symbol]
	if !ok {
		clients = make(map[*ClientState]struct{})
		m.clientsBySymbol[symbol] = clients
	}
	isNew := len(clients) == 0
	clients[client] = struct{}{}
	return isNew
}

// true if last subscriber
func (m *SubscriptionManager) Unsubscribe(symbol string, client *ClientState) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	clients, ok := m.clientsBySymbol[symbol]
	if !ok {
		return false
	}
	if _, present := clients[client]; !present {
		return false
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(m.clientsBySymbol, symbol)
		return true
	}
	return false
}

func (m *SubscriptionManager) UnsubscribeAll(client *ClientState) []string {
	m.mu.Lock()
	symbols := make([]string, 0, len(m.clientsBySymbol))
	for symbol := range m.clientsBySymbol {
		symbols = append(symbols, symbol)
	}
	m.mu.Unlock()

	var emptied []string
	for _, symbol := range symbols {
		if m.Unsubscribe(symbol, client) {
			emptied = append(emptied, symbol)
		}
	}
	return emptied
}

func (m *SubscriptionManager) ClientsFor(symbol string) []*ClientState {
	m.mu.Lock()
	defer m.mu.Unlock()
	clients := m.clientsBySymbol[symbol]
	result := make([]*ClientState, 0, len(clients))
	for c := range clients {
		result = append(result, c)
	}
	return result
}
