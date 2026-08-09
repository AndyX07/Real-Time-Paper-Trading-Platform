package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"papertrader/backend/internal/book"
	"papertrader/backend/internal/candle"
	"papertrader/backend/internal/config"
	"papertrader/backend/internal/control"
	"papertrader/backend/internal/persistence"
	"papertrader/backend/internal/symbols"
	"papertrader/backend/internal/trading"
)

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://"+config.DevOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	engineClient := control.New()
	if err := engineClient.Start(); err != nil {
		slog.Error("main: engine client failed to start", "error", err)
		os.Exit(1)
	}
	defer engineClient.Close()

	candleRouter := candle.NewRouter()
	candleRouter.Start(ctx)
	defer candleRouter.Stop()

	bookRouter := book.NewRouter(engineClient)
	bookRouter.Start(ctx)
	defer bookRouter.Stop()

	db, err := persistence.Open(config.DBPath())
	if err != nil {
		slog.Error("main: failed to open persistence db", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	repo := persistence.NewRepository(db)

	tradingRouter := trading.NewRouter(engineClient, repo)
	tradingRouter.Start(ctx)
	defer tradingRouter.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/debug/book_counters", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bookRouter.Counters().Snapshot())
	})
	mux.HandleFunc("/api/symbols", symbols.Handler)
	mux.HandleFunc("/api/candles/history", candleRouter.HandleHistory)
	mux.HandleFunc("/ws/candles", candleRouter.HandleWS)
	mux.HandleFunc("/ws/book", bookRouter.HandleWS)
	mux.HandleFunc("/ws/control", tradingRouter.HandleWS)

	addr := config.Host() + ":" + config.Port()
	server := &http.Server{Addr: addr, Handler: withCORS(mux)}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	slog.Info("main: listening", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("main: server error", "error", err)
		os.Exit(1)
	}
}
