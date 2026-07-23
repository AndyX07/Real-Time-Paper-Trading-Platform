package candle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"papertrader/backend/internal/config"
)

type HistoryBar struct {
	Time  int64   `json:"time"`
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
}

type ohlcHistoryResponse struct {
	Error  []string                   `json:"error"`
	Result map[string]json.RawMessage `json:"result"`
}

var historyHTTPClient = &http.Client{Timeout: 10 * time.Second}

func FetchOhlcHistory(ctx context.Context, symbol string, intervalMinutes, count int) ([]HistoryBar, error) {
	query := url.Values{}
	query.Set("pair", symbol)
	query.Set("interval", strconv.Itoa(intervalMinutes))
	reqURL := config.KrakenRestOhlcURL + "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := historyHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("candle.history: unexpected status %d", resp.StatusCode)
	}

	var payload ohlcHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Error) > 0 {
		return nil, fmt.Errorf("Kraken OHLC history error: %v", payload.Error)
	}

	raw, ok := payload.Result[symbol]
	if !ok {
		for key, value := range payload.Result {
			if key != "last" {
				raw = value
				ok = true
				break
			}
		}
	}
	if !ok {
		return []HistoryBar{}, nil
	}

	var rows [][]json.Number
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}

	bars := make([]HistoryBar, 0, len(rows))
	for _, row := range rows {
		if len(row) < 5 {
			continue
		}
		t, _ := row[0].Int64()
		open, _ := row[1].Float64()
		high, _ := row[2].Float64()
		low, _ := row[3].Float64()
		closePrice, _ := row[4].Float64()
		bars = append(bars, HistoryBar{Time: t, Open: open, High: high, Low: low, Close: closePrice})
	}

	if count > 0 && len(bars) > count {
		bars = bars[len(bars)-count:]
	}
	return bars, nil
}
