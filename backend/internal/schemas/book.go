package schemas

type PriceLevelMessage struct {
	PriceTicks int64 `json:"priceTicks"`
	SizeTicks  int64 `json:"sizeTicks"`
}

type BookDeltaMessage struct {
	Type       string `json:"type"`
	Symbol     string `json:"symbol"`
	Seq        uint64 `json:"seq"`
	Side       string `json:"side"` // "bid" | "ask"
	PriceTicks int64  `json:"priceTicks"`
	SizeTicks  int64  `json:"sizeTicks"`
}

func NewBookDeltaMessage(symbol string, seq uint64, side string, priceTicks, sizeTicks int64) BookDeltaMessage {
	return BookDeltaMessage{
		Type: "book_delta", Symbol: symbol, Seq: seq, Side: side,
		PriceTicks: priceTicks, SizeTicks: sizeTicks,
	}
}

type BookSnapshotMessage struct {
	Type   string              `json:"type"`
	Symbol string              `json:"symbol"`
	Seq    uint64              `json:"seq"`
	Bids   []PriceLevelMessage `json:"bids"`
	Asks   []PriceLevelMessage `json:"asks"`
}

func NewBookSnapshotMessage(symbol string, seq uint64, bids, asks []PriceLevelMessage) BookSnapshotMessage {
	return BookSnapshotMessage{Type: "book_snapshot", Symbol: symbol, Seq: seq, Bids: bids, Asks: asks}
}
