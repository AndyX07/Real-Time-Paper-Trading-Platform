package schemas

type PlaceOrderMessage struct {
	Type            string `json:"type"`
	Symbol          string `json:"symbol"`
	Side            string `json:"side"`      // "buy" | "sell"
	OrderType       string `json:"orderType"` // "market" | "limit"
	Price           string `json:"price"`
	Size            string `json:"size"`
	ClientRequestID string `json:"clientRequestId"`
}

type CancelOrderMessage struct {
	Type            string `json:"type"`
	OrderID         int64  `json:"orderId"`
	ClientRequestID string `json:"clientRequestId"`
}

type OrderAckMessage struct {
	Type            string `json:"type"`
	ClientRequestID string `json:"clientRequestId"`
	OrderID         int64  `json:"orderId"`
	Status          string `json:"status"` // "accepted" | "rejected"
	Reason          string `json:"reason,omitempty"`
}

func NewOrderAckMessage(clientRequestID string, orderID int64, status, reason string) OrderAckMessage {
	return OrderAckMessage{
		Type: "order_ack", ClientRequestID: clientRequestID, OrderID: orderID, Status: status, Reason: reason,
	}
}

type FillEventMessage struct {
	Type       string `json:"type"`
	OrderID    int64  `json:"orderId"`
	PriceTicks int64  `json:"priceTicks"`
	SizeTicks  int64  `json:"sizeTicks"`
	Ts         uint64 `json:"ts"`
}

func NewFillEventMessage(orderID int64, priceTicks, sizeTicks int64, ts uint64) FillEventMessage {
	return FillEventMessage{Type: "fill_event", OrderID: orderID, PriceTicks: priceTicks, SizeTicks: sizeTicks, Ts: ts}
}

type OrderSnapshot struct {
	OrderID       int64  `json:"orderId"`
	EngineOrderID *int64 `json:"engineOrderId,omitempty"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	OrderType     string `json:"orderType"`
	PriceTicks    *int64 `json:"priceTicks,omitempty"`
	SizeTicks     int64  `json:"sizeTicks"`
	Status        string `json:"status"`
	CancelReason  string `json:"cancelReason,omitempty"`
	RejectReason  string `json:"rejectReason,omitempty"`
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
}

type PositionSnapshot struct {
	Symbol           string `json:"symbol"`
	NetSizeTicks     int64  `json:"netSizeTicks"`
	AvgCostTicks     int64  `json:"avgCostTicks"`
	RealizedPnLTicks int64  `json:"realizedPnlTicks"`
}

type OrderUpdateMessage struct {
	Type         string `json:"type"`
	OrderID      int64  `json:"orderId"`
	Status       string `json:"status"`
	CancelReason string `json:"cancelReason,omitempty"`
}

func NewOrderUpdateMessage(orderID int64, status, cancelReason string) OrderUpdateMessage {
	return OrderUpdateMessage{Type: "order_update", OrderID: orderID, Status: status, CancelReason: cancelReason}
}

type StateSnapshotMessage struct {
	Type      string             `json:"type"`
	Orders    []OrderSnapshot    `json:"orders"`
	Positions []PositionSnapshot `json:"positions"`
}

func NewStateSnapshotMessage(orders []OrderSnapshot, positions []PositionSnapshot) StateSnapshotMessage {
	return StateSnapshotMessage{Type: "state_snapshot", Orders: orders, Positions: positions}
}
