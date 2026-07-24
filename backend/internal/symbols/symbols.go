package symbols

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"papertrader/backend/internal/config"
)

var legacyTickerAliases = map[string]string{
	"XBT": "BTC",
}

type assetPairsResponse struct {
	Error  []string                   `json:"error"`
	Result map[string]krakenAssetPair `json:"result"`
}

type krakenAssetPair struct {
	WsName       string `json:"wsname"`
	Status       string `json:"status"`
	PairDecimals int    `json:"pair_decimals"`
	LotDecimals  int    `json:"lot_decimals"`
}

type SymbolInfo struct {
	Symbol           string `json:"symbol"`
	PriceDecimals    int    `json:"priceDecimals"`
	QuantityDecimals int    `json:"quantityDecimals"`
}

var (
	mu      sync.Mutex
	symbols []SymbolInfo
)

func fetchSymbols() ([]SymbolInfo, error) {
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(config.KrakenRestAssetPairsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("symbols: kraken AssetPairs returned %s", resp.Status)
	}

	var parsed assetPairsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(parsed.Result))
	out := make([]SymbolInfo, 0, len(parsed.Result))
	for _, pair := range parsed.Result {
		if pair.Status != "online" || !strings.HasSuffix(pair.WsName, "/USD") {
			continue
		}
		symbol := translate(pair.WsName)
		if _, dup := seen[symbol]; dup {
			continue
		}
		seen[symbol] = struct{}{}
		out = append(out, SymbolInfo{Symbol: symbol, PriceDecimals: pair.PairDecimals, QuantityDecimals: pair.LotDecimals})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out, nil
}

func translate(wsname string) string {
	base, quote, ok := strings.Cut(wsname, "/")
	if !ok {
		return wsname
	}
	if alias, ok := legacyTickerAliases[base]; ok {
		base = alias
	}
	return base + "/" + quote
}

func Handler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	if symbols == nil {
		fetched, err := fetchSymbols()
		if err != nil {
			http.Error(w, "failed to fetch symbol list", http.StatusServiceUnavailable)
			return
		}
		symbols = fetched
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(symbols)
}
