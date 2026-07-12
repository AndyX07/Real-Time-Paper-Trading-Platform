import { useState } from "react";
import { CandleChart } from "./components/CandleChart/CandleChart";
import { OrderEntry, type FakeOrder } from "./components/OrderEntry/OrderEntry";
import { OpenOrders } from "./components/OpenOrders/OpenOrders";

const SYMBOLS = ["BTC/USD", "ETH/USD", "SOL/USD"];

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
  const [symbol, setSymbol] = useState(SYMBOLS[0]);
  const [intervalMinutes, setIntervalMinutes] = useState(INTERVALS[0].minutes);
  const [orders, setOrders] = useState<FakeOrder[]>([]);

  return (
    <div style={{ fontFamily: "sans-serif", color: "#d1d4dc", background: "#0d1117", minHeight: "100vh", padding: 16 }}>
      <h1 style={{ fontSize: 18 }}>Paper Trader</h1>

      <div style={{ marginBottom: 8 }}>
        {SYMBOLS.map((s) => (
          <button
            key={s}
            onClick={() => setSymbol(s)}
            style={{
              marginRight: 8,
              padding: "6px 12px",
              background: s === symbol ? "#26a69a" : "#1e222d",
              color: "#fff",
              border: "none",
              borderRadius: 4,
              cursor: "pointer",
            }}
          >
            {s}
          </button>
        ))}
      </div>

      <div style={{ marginBottom: 12 }}>
        {INTERVALS.map((iv) => (
          <button
            key={iv.minutes}
            onClick={() => setIntervalMinutes(iv.minutes)}
            style={{
              marginRight: 6,
              padding: "4px 10px",
              fontSize: 12,
              background: iv.minutes === intervalMinutes ? "#3f51b5" : "#1e222d",
              color: "#fff",
              border: "none",
              borderRadius: 4,
              cursor: "pointer",
            }}
          >
            {iv.label}
          </button>
        ))}
      </div>

      <CandleChart symbol={symbol} intervalMinutes={intervalMinutes} />

      <div style={{ display: "flex", gap: 24, marginTop: 16 }}>
        <OrderEntry symbol={symbol} onOrderFilled={(order) => setOrders((prev) => [order, ...prev])} />
        <OpenOrders orders={orders} />
      </div>
    </div>
  );
}
