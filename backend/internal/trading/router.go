package trading

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"papertrader/backend/internal/config"
	"papertrader/backend/internal/control"
	"papertrader/backend/internal/persistence"
	"papertrader/backend/internal/schemas"
)

type Router struct {
	engineClient *control.EngineClient
	repo         *persistence.Repository
	clients      *ClientRegistry
	fillWatcher  *FillWatcher
}

func NewRouter(engineClient *control.EngineClient, repo *persistence.Repository) *Router {
	clients := NewClientRegistry()
	return &Router{
		engineClient: engineClient,
		repo:         repo,
		clients:      clients,
		fillWatcher:  NewFillWatcher(engineClient, repo, clients),
	}
}

func (r *Router) Start(ctx context.Context) {
	go r.fillWatcher.Run(ctx)
}

func (r *Router) Stop() {
	r.fillWatcher.Stop()
}

// incomingMessage covers both place_order and cancel_order -- their field
// sets don't overlap much, but declaring the union once and letting
// unused fields sit zero-valued is simpler than a two-pass decode.
type incomingMessage struct {
	Type            string `json:"type"`
	Symbol          string `json:"symbol"`
	Side            string `json:"side"`
	OrderType       string `json:"orderType"`
	Price           string `json:"price"`
	Size            string `json:"size"`
	OrderID         int64  `json:"orderId"`
	ClientRequestID string `json:"clientRequestId"`
}

func (r *Router) HandleWS(w http.ResponseWriter, req *http.Request) {
	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{OriginPatterns: []string{config.DevOrigin}})
	if err != nil {
		slog.Error("trading.router: accept failed", "error", err)
		return
	}
	ctx := req.Context()
	client := NewClientState(conn)
	r.clients.Add(client)

	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		r.clientSender(ctx, client)
	}()

	r.sendInitialSnapshot(client)

	defer func() {
		conn.CloseNow()
		<-senderDone
		r.clients.Remove(client)
	}()

	for {
		var msg incomingMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return
		}
		switch msg.Type {
		case "place_order":
			r.handlePlaceOrder(ctx, msg)
		case "cancel_order":
			r.handleCancelOrder(ctx, msg)
		}
	}
}

func (r *Router) clientSender(ctx context.Context, client *ClientState) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-client.Outbox:
			if err := wsjson.Write(ctx, client.Conn, msg); err != nil {
				return
			}
		}
	}
}

func (r *Router) sendInitialSnapshot(client *ClientState) {
	orders, err := r.repo.GetAllOrders()
	if err != nil {
		slog.Error("trading.router: get all orders failed", "error", err)
	}
	positions, err := r.repo.GetPositions()
	if err != nil {
		slog.Error("trading.router: get positions failed", "error", err)
	}

	orderSnapshots := make([]schemas.OrderSnapshot, len(orders))
	for i, o := range orders {
		orderSnapshots[i] = toOrderSnapshot(o)
	}

	positionSnapshots := make([]schemas.PositionSnapshot, len(positions))
	for i, p := range positions {
		positionSnapshots[i] = schemas.PositionSnapshot{
			Symbol: p.Symbol, NetSizeTicks: p.NetSizeTicks, AvgCostTicks: p.AvgCostTicks,
			RealizedPnLTicks: p.RealizedPnLTicks,
		}
	}

	client.Enqueue(schemas.NewStateSnapshotMessage(orderSnapshots, positionSnapshots))
}

func toOrderSnapshot(o persistence.Order) schemas.OrderSnapshot {
	var engineOrderID *int64
	if o.EngineOrderID.Valid {
		v := o.EngineOrderID.Int64
		engineOrderID = &v
	}
	var priceTicks *int64
	if o.PriceTicks.Valid {
		v := o.PriceTicks.Int64
		priceTicks = &v
	}
	return schemas.OrderSnapshot{
		OrderID: o.OrderID, EngineOrderID: engineOrderID, Symbol: o.Symbol, Side: o.Side, OrderType: o.OrderType,
		PriceTicks: priceTicks, SizeTicks: o.SizeTicks, Status: o.Status,
		CancelReason: o.CancelReason.String, RejectReason: o.RejectReason.String,
		CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}

func (r *Router) handlePlaceOrder(ctx context.Context, msg incomingMessage) {
	// engine_client.go's side/orderType -> proto enum mapping silently
	// defaults anything unrecognized to buy/limit -- validate here instead
	// of letting a typo or malformed message become a real, unintended
	// trading decision.
	if msg.Side != "buy" && msg.Side != "sell" {
		r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, 0, "rejected",
			fmt.Sprintf("invalid side: %q", msg.Side)))
		return
	}
	if msg.OrderType != "market" && msg.OrderType != "limit" {
		r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, 0, "rejected",
			fmt.Sprintf("invalid orderType: %q", msg.OrderType)))
		return
	}

	var priceTicksPtr *int64
	var priceTicks int64
	if msg.OrderType == "limit" {
		parsed, err := parseTicks(msg.Price)
		if err != nil {
			r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, 0, "rejected",
				fmt.Sprintf("invalid price: %v", err)))
			return
		}
		priceTicks = parsed
		priceTicksPtr = &priceTicks
	}

	sizeTicks, err := parseTicks(msg.Size)
	if err != nil {
		r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, 0, "rejected",
			fmt.Sprintf("invalid size: %v", err)))
		return
	}

	orderID, err := r.repo.CreatePendingOrder(msg.Symbol, msg.Side, msg.OrderType, priceTicksPtr, sizeTicks,
		msg.ClientRequestID)
	if err != nil {
		slog.Error("trading.router: create pending order failed", "error", err)
		r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, 0, "rejected", "internal error"))
		return
	}

	result := r.engineClient.PlaceOrder(ctx, msg.Symbol, msg.Side, msg.OrderType, priceTicks, sizeTicks,
		msg.ClientRequestID)
	if !result.Accepted {
		if err := r.repo.MarkOrderRejected(orderID, result.RejectReason); err != nil {
			slog.Error("trading.router: mark order rejected failed", "orderId", orderID, "error", err)
		}
		r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, orderID, "rejected", result.RejectReason))
		return
	}

	if err := r.repo.MarkOrderAccepted(orderID, result.EngineOrderID); err != nil {
		slog.Error("trading.router: mark order accepted failed", "orderId", orderID, "error", err)
	}
	r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, orderID, "accepted", ""))
}

func (r *Router) handleCancelOrder(ctx context.Context, msg incomingMessage) {
	order, err := r.repo.GetOrder(msg.OrderID)
	if err != nil {
		r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, msg.OrderID, "rejected", "order not found"))
		return
	}
	if !order.EngineOrderID.Valid {
		r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, msg.OrderID, "rejected",
			"order was never accepted"))
		return
	}

	engineOrderID := uint64(order.EngineOrderID.Int64)
	result := r.engineClient.CancelOrder(ctx, engineOrderID)
	if !result.Accepted {
		r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, msg.OrderID, "rejected", result.RejectReason))
		return
	}

	if err := r.repo.MarkOrderCanceled(msg.OrderID, "user"); err != nil {
		slog.Error("trading.router: mark order canceled failed", "orderId", msg.OrderID, "error", err)
	}
	r.clients.Broadcast(schemas.NewOrderAckMessage(msg.ClientRequestID, msg.OrderID, "accepted", ""))
}
