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
