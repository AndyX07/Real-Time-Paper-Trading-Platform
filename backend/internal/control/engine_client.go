package control

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "papertrader/backend/genproto"
)

// OrderResult mirrors OrderReply (paper_trader.proto) -- shared shape for
// both PlaceOrder and CancelOrder replies, same as the engine's own
// OrderResult that produces it.
type OrderResult struct {
	Accepted        bool
	EngineOrderID   uint64
	RejectReason    string
	FilledSizeTicks int64
}

func engineAddress() string {
	host := os.Getenv("ENGINE_GRPC_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("ENGINE_GRPC_PORT")
	if port == "" {
		port = "50051"
	}
	return fmt.Sprintf("%s:%s", host, port)
}

type SubscribeResult struct {
	Ok     bool
	Reason string
}

type EngineClient struct {
	conn   *grpc.ClientConn
	client pb.EngineControlClient
}

func New() *EngineClient {
	return &EngineClient{}
}

func (e *EngineClient) Start() error {
	address := engineAddress()
	// plaintext, no tls for local dev
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("control.engine_client: dial %s: %w", address, err)
	}
	e.conn = conn
	e.client = pb.NewEngineControlClient(conn)
	slog.Info("control.engine_client targeting", "address", address)
	return nil
}

func (e *EngineClient) Close() error {
	if e.conn == nil {
		return nil
	}
	err := e.conn.Close()
	e.conn = nil
	e.client = nil
	return err
}

func (e *EngineClient) SubscribeBook(ctx context.Context, symbol string) SubscribeResult {
	reply, err := e.client.SubscribeBook(ctx, &pb.SubscribeRequest{Symbol: symbol})
	if err != nil {
		slog.Error("control.engine_client RPC failed", "symbol", symbol, "error", err)
		return SubscribeResult{Ok: false, Reason: fmt.Sprintf("engine unreachable: %v", err)}
	}
	return SubscribeResult{Ok: reply.GetOk(), Reason: reply.GetReason()}
}

func (e *EngineClient) UnsubscribeBook(ctx context.Context, symbol string) SubscribeResult {
	reply, err := e.client.UnsubscribeBook(ctx, &pb.SubscribeRequest{Symbol: symbol})
	if err != nil {
		slog.Error("control.engine_client RPC failed", "symbol", symbol, "error", err)
		return SubscribeResult{Ok: false, Reason: fmt.Sprintf("engine unreachable: %v", err)}
	}
	return SubscribeResult{Ok: reply.GetOk(), Reason: reply.GetReason()}
}

func (e *EngineClient) PlaceOrder(ctx context.Context, symbol, side, orderType string, priceTicks, sizeTicks int64,
	clientRequestID string) OrderResult {
	pbSide := pb.Side_BID
	if side == "sell" {
		pbSide = pb.Side_ASK
	}
	pbType := pb.OrderType_LIMIT
	if orderType == "market" {
		pbType = pb.OrderType_MARKET
	}

	reply, err := e.client.PlaceOrder(ctx, &pb.PlaceOrderRequest{
		Symbol: symbol, Side: pbSide, Type: pbType, PriceTicks: priceTicks, SizeTicks: sizeTicks,
		ClientRequestId: clientRequestID,
	})
	if err != nil {
		slog.Error("control.engine_client RPC failed", "rpc", "PlaceOrder", "symbol", symbol, "error", err)
		return OrderResult{Accepted: false, RejectReason: fmt.Sprintf("engine unreachable: %v", err)}
	}
	return OrderResult{Accepted: reply.GetAccepted(), EngineOrderID: reply.GetEngineOrderId(),
		RejectReason: reply.GetRejectReason(), FilledSizeTicks: reply.GetFilledSizeTicks()}
}

func (e *EngineClient) CancelOrder(ctx context.Context, engineOrderID uint64) OrderResult {
	reply, err := e.client.CancelOrder(ctx, &pb.CancelOrderRequest{EngineOrderId: engineOrderID})
	if err != nil {
		slog.Error("control.engine_client RPC failed", "rpc", "CancelOrder", "error", err)
		return OrderResult{Accepted: false, RejectReason: fmt.Sprintf("engine unreachable: %v", err)}
	}
	return OrderResult{Accepted: reply.GetAccepted(), EngineOrderID: reply.GetEngineOrderId(),
		RejectReason: reply.GetRejectReason()}
}

func (e *EngineClient) WatchFills(ctx context.Context) (grpc.ServerStreamingClient[pb.FillEvent], error) {
	return e.client.WatchFills(ctx, &pb.WatchFillsRequest{})
}

func (e *EngineClient) GetEngineInfo(ctx context.Context) (uint64, error) {
	reply, err := e.client.GetEngineInfo(ctx, &pb.EngineInfoRequest{})
	if err != nil {
		return 0, fmt.Errorf("control.engine_client: GetEngineInfo: %w", err)
	}
	return reply.GetInstanceId(), nil
}
