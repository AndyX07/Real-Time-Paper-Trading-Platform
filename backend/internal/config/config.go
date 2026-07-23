package config

import "os"

const (
	KrakenOhlcWSURL    = "wss://ws.kraken.com/v2"
	KrakenRestOhlcURL  = "https://api.kraken.com/0/public/OHLC"
	DefaultOhlcMinutes = 1
	DevOrigin          = "localhost:5173"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func Host() string { return getenv("HOST", "0.0.0.0") }
func Port() string { return getenv("PORT", "8000") }
