package candle

import "sync"

type SubKey struct {
	Symbol          string
	IntervalMinutes int
}

type SubscriptionManager struct {
	mu           sync.Mutex
	clientsByKey map[SubKey]map[*ClientState]struct{}
}

func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{clientsByKey: make(map[SubKey]map[*ClientState]struct{})}
}

// returns true if this is first subscriber
func (m *SubscriptionManager) Subscribe(symbol string, intervalMinutes int, client *ClientState) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := SubKey{symbol, intervalMinutes}
	clients, ok := m.clientsByKey[key]
	if !ok {
		clients = make(map[*ClientState]struct{})
		m.clientsByKey[key] = clients
	}
	isNew := len(clients) == 0
	clients[client] = struct{}{}
	return isNew
}

// returns true if this was the last subscriber
func (m *SubscriptionManager) Unsubscribe(symbol string, intervalMinutes int, client *ClientState) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := SubKey{symbol, intervalMinutes}
	clients, ok := m.clientsByKey[key]
	if !ok {
		return false
	}
	if _, present := clients[client]; !present {
		return false
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(m.clientsByKey, key)
		return true
	}
	return false
}

// returns keys that got unsubscribed
func (m *SubscriptionManager) UnsubscribeAll(client *ClientState) []SubKey {
	m.mu.Lock()
	keys := make([]SubKey, 0, len(m.clientsByKey))
	for key := range m.clientsByKey {
		keys = append(keys, key)
	}
	m.mu.Unlock()

	var emptied []SubKey
	for _, key := range keys {
		if m.Unsubscribe(key.Symbol, key.IntervalMinutes, client) {
			emptied = append(emptied, key)
		}
	}
	return emptied
}

func (m *SubscriptionManager) ClientsFor(symbol string, intervalMinutes int) []*ClientState {
	m.mu.Lock()
	defer m.mu.Unlock()
	clients := m.clientsByKey[SubKey{symbol, intervalMinutes}]
	result := make([]*ClientState, 0, len(clients))
	for c := range clients {
		result = append(result, c)
	}
	return result
}
