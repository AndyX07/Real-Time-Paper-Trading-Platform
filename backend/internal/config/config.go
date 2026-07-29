package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	KrakenOhlcWSURL         = "wss://ws.kraken.com/v2"
	KrakenRestOhlcURL       = "https://api.kraken.com/0/public/OHLC"
	KrakenRestAssetPairsURL = "https://api.kraken.com/0/public/AssetPairs"
	DefaultOhlcMinutes      = 1
	DevOrigin               = "localhost:5173"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func Host() string { return getenv("HOST", "0.0.0.0") }
func Port() string { return getenv("PORT", "8000") }

func dbPathDefault() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "paper_trader.db" // fallback
	}
	backendRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile))) // config.go -> internal -> backend
	return filepath.Join(backendRoot, "paper_trader.db")
}

func DBPath() string { return getenv("DB_PATH", dbPathDefault()) }
