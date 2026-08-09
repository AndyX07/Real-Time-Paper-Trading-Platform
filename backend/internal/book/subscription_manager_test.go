package book

import (
	"sync"
	"testing"
)

func TestSubscriptionManagerFirstAndSecondSubscribe(t *testing.T) {
	m := NewSubscriptionManager()
	c1 := NewClientState(nil)
	c2 := NewClientState(nil)

	if isNew := m.Subscribe("BTC/USD", c1); !isNew {
		t.Fatalf("first subscribe should report isNew=true")
	}
	if isNew := m.Subscribe("BTC/USD", c2); isNew {
		t.Fatalf("second subscribe should report isNew=false")
	}
}

func TestSubscriptionManagerLastUnsubscribe(t *testing.T) {
	m := NewSubscriptionManager()
	c1 := NewClientState(nil)
	c2 := NewClientState(nil)
	m.Subscribe("BTC/USD", c1)
	m.Subscribe("BTC/USD", c2)

	if wasLast := m.Unsubscribe("BTC/USD", c1); wasLast {
		t.Fatalf("unsubscribing non-last client should report wasLast=false")
	}
	if wasLast := m.Unsubscribe("BTC/USD", c2); !wasLast {
		t.Fatalf("unsubscribing last client should report wasLast=true")
	}
}

func TestSubscriptionManagerUnsubscribeAllReturnsOnlyEmptied(t *testing.T) {
	m := NewSubscriptionManager()
	c1 := NewClientState(nil)
	c2 := NewClientState(nil)
	m.Subscribe("BTC/USD", c1)
	m.Subscribe("BTC/USD", c2) // BTC/USD has two subscribers, won't empty when c1 leaves
	m.Subscribe("ETH/USD", c1) // ETH/USD has only c1, will empty

	emptied := m.UnsubscribeAll(c1)
	if len(emptied) != 1 || emptied[0] != "ETH/USD" {
		t.Fatalf("UnsubscribeAll(c1) = %v, want [ETH/USD]", emptied)
	}
	if got := m.ClientsFor("BTC/USD"); len(got) != 1 || got[0] != c2 {
		t.Fatalf("BTC/USD should still have c2 subscribed, got %v", got)
	}
}

func TestSubscriptionManagerClientsForIsolation(t *testing.T) {
	m := NewSubscriptionManager()
	c1 := NewClientState(nil)
	c2 := NewClientState(nil)
	m.Subscribe("BTC/USD", c1)
	m.Subscribe("ETH/USD", c2)

	btc := m.ClientsFor("BTC/USD")
	if len(btc) != 1 || btc[0] != c1 {
		t.Fatalf("ClientsFor(BTC/USD) = %v, want [c1]", btc)
	}
	eth := m.ClientsFor("ETH/USD")
	if len(eth) != 1 || eth[0] != c2 {
		t.Fatalf("ClientsFor(ETH/USD) = %v, want [c2]", eth)
	}
}

func TestSubscriptionManagerConcurrentSubscribeUnsubscribe(t *testing.T) {
	m := NewSubscriptionManager()
	clients := make([]*ClientState, 50)
	for i := range clients {
		clients[i] = NewClientState(nil)
	}

	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(c *ClientState) {
			defer wg.Done()
			m.Subscribe("BTC/USD", c)
			m.Unsubscribe("BTC/USD", c)
		}(c)
	}
	wg.Wait()

	if got := m.ClientsFor("BTC/USD"); len(got) != 0 {
		t.Fatalf("ClientsFor(BTC/USD) after all subscribe+unsubscribe = %v, want empty", got)
	}
}
