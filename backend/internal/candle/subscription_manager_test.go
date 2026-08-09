package candle

import "testing"

func TestSubscriptionManagerFirstAndSecondSubscribe(t *testing.T) {
	m := NewSubscriptionManager()
	c1 := NewClientState(nil)
	c2 := NewClientState(nil)

	if isNew := m.Subscribe("BTC/USD", 1, c1); !isNew {
		t.Fatalf("first subscribe should report isNew=true")
	}
	if isNew := m.Subscribe("BTC/USD", 1, c2); isNew {
		t.Fatalf("second subscribe should report isNew=false")
	}
}

func TestSubscriptionManagerLastUnsubscribe(t *testing.T) {
	m := NewSubscriptionManager()
	c1 := NewClientState(nil)
	c2 := NewClientState(nil)
	m.Subscribe("BTC/USD", 1, c1)
	m.Subscribe("BTC/USD", 1, c2)

	if wasLast := m.Unsubscribe("BTC/USD", 1, c1); wasLast {
		t.Fatalf("unsubscribing non-last client should report wasLast=false")
	}
	if wasLast := m.Unsubscribe("BTC/USD", 1, c2); !wasLast {
		t.Fatalf("unsubscribing last client should report wasLast=true")
	}
}

func TestSubscriptionManagerUnsubscribeAllReturnsOnlyEmptied(t *testing.T) {
	m := NewSubscriptionManager()
	c1 := NewClientState(nil)
	c2 := NewClientState(nil)
	m.Subscribe("BTC/USD", 1, c1)
	m.Subscribe("BTC/USD", 1, c2) // won't empty when c1 leaves
	m.Subscribe("ETH/USD", 1, c1) // will empty

	emptied := m.UnsubscribeAll(c1)
	if len(emptied) != 1 || emptied[0] != (SubKey{"ETH/USD", 1}) {
		t.Fatalf("UnsubscribeAll(c1) = %v, want [{ETH/USD 1}]", emptied)
	}
}

func TestSubscriptionManagerClientsForIsolation(t *testing.T) {
	m := NewSubscriptionManager()
	c1 := NewClientState(nil)
	c2 := NewClientState(nil)
	m.Subscribe("BTC/USD", 1, c1)
	m.Subscribe("ETH/USD", 1, c2)

	if got := m.ClientsFor("BTC/USD", 1); len(got) != 1 || got[0] != c1 {
		t.Fatalf("ClientsFor(BTC/USD, 1) = %v, want [c1]", got)
	}
	if got := m.ClientsFor("ETH/USD", 1); len(got) != 1 || got[0] != c2 {
		t.Fatalf("ClientsFor(ETH/USD, 1) = %v, want [c2]", got)
	}
}

// The same symbol at two different intervals must be tracked as
// independent refcount buckets -- subscribing/unsubscribing one interval
// must not affect the other.
func TestSubscriptionManagerSameSymbolDifferentIntervalsAreIndependent(t *testing.T) {
	m := NewSubscriptionManager()
	c1 := NewClientState(nil)
	c2 := NewClientState(nil)

	m.Subscribe("BTC/USD", 1, c1)
	m.Subscribe("BTC/USD", 60, c2)

	if got := m.ClientsFor("BTC/USD", 1); len(got) != 1 || got[0] != c1 {
		t.Fatalf("ClientsFor(BTC/USD, 1) = %v, want [c1]", got)
	}
	if got := m.ClientsFor("BTC/USD", 60); len(got) != 1 || got[0] != c2 {
		t.Fatalf("ClientsFor(BTC/USD, 60) = %v, want [c2]", got)
	}

	if wasLast := m.Unsubscribe("BTC/USD", 1, c1); !wasLast {
		t.Fatalf("unsubscribing the only 1m subscriber should report wasLast=true")
	}
	if got := m.ClientsFor("BTC/USD", 60); len(got) != 1 || got[0] != c2 {
		t.Fatalf("unsubscribing the 1m interval must not affect the 60m interval, got %v", got)
	}
}
