import { useEffect, useState } from "react";
import { CandleChart } from "./components/CandleChart/CandleChart";
import { OrderBookLadder } from "./components/OrderBookLadder/OrderBookLadder";
import { OrderEntry, type FakeOrder } from "./components/OrderEntry/OrderEntry";
import { BottomPanel } from "./components/BottomPanel/BottomPanel";
import { SymbolPicker } from "./components/SymbolPicker/SymbolPicker";
import type { SymbolInfo } from "./types/symbol";

const DEFAULT_SYMBOL = "BTC/USD";
const DEFAULT_PRICE_DECIMALS = 2;
const DEFAULT_QUANTITY_DECIMALS = 8;
const SYMBOLS_URL = "http://localhost:8000/api/symbols";

// Kraken-valid OHLC intervals (minutes), curated to a reasonable UI subset.
const INTERVALS = [
  { label: "1m", minutes: 1 },
  { label: "5m", minutes: 5 },
  { label: "15m", minutes: 15 },
  { label: "1h", minutes: 60 },
  { label: "4h", minutes: 240 },
  { label: "1d", minutes: 1440 },
];

export function App() {
  const [symbol, setSymbol] = useState(DEFAULT_SYMBOL);
  const [intervalMinutes, setIntervalMinutes] = useState(INTERVALS[0].minutes);
  const [orders, setOrders] = useState<FakeOrder[]>([]);
  const [symbolInfos, setSymbolInfos] = useState<SymbolInfo[]>([]);

  // Fetched once here (rather than inside SymbolPicker) since price
  // precision -- not just the searchable list -- is derived from the same
  // response and needed by the chart/ladder too.
  useEffect(() => {
    let cancelled = false;
    fetch(SYMBOLS_URL)
      .then((res) => (res.ok ? res.json() : []))
      .then((list: SymbolInfo[]) => {
        if (!cancelled) setSymbolInfos(list);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  const currentSymbolInfo = symbolInfos.find((s) => s.symbol === symbol);
  const priceDecimals = currentSymbolInfo?.priceDecimals ?? DEFAULT_PRICE_DECIMALS;
  const quantityDecimals = currentSymbolInfo?.quantityDecimals ?? DEFAULT_QUANTITY_DECIMALS;

  return (
    <div className="flex h-screen flex-col gap-2 bg-app-bg p-2 font-sans text-sm text-text-primary">
      <header className="flex items-center gap-3 px-1">
        <h1 className="text-base font-bold tracking-tight">Paper Trader</h1>
        <SymbolPicker symbol={symbol} symbols={symbolInfos.map((s) => s.symbol)} onSelect={setSymbol} />
      </header>

      <div className="flex min-h-0 flex-1 gap-2 overflow-hidden">
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-panel">
          <div className="flex items-center gap-1 border-b border-border px-3 py-1.5">
            {INTERVALS.map((iv) => (
              <button
                key={iv.minutes}
                onClick={() => setIntervalMinutes(iv.minutes)}
                className={`rounded px-2.5 py-1 text-xs font-medium transition-colors ${
                  iv.minutes === intervalMinutes
                    ? "bg-panel-alt text-text-primary"
                    : "text-text-muted hover:text-text-primary"
                }`}
              >
                {iv.label}
              </button>
            ))}
          </div>
          <div className="min-h-0 flex-1 p-2">
            <CandleChart symbol={symbol} intervalMinutes={intervalMinutes} priceDecimals={priceDecimals} />
          </div>
        </div>

        <div className="min-h-0 w-72 shrink-0 overflow-hidden rounded-lg border border-border">
          <OrderBookLadder symbol={symbol} priceDecimals={priceDecimals} quantityDecimals={quantityDecimals} />
        </div>

        <div className="min-h-0 w-80 shrink-0 overflow-hidden rounded-lg border border-border">
          <OrderEntry symbol={symbol} onOrderFilled={(order) => setOrders((prev) => [order, ...prev])} />
        </div>
      </div>

      <div className="shrink-0 overflow-hidden rounded-lg border border-border">
        <BottomPanel orders={orders} />
      </div>
    </div>
  );
}
