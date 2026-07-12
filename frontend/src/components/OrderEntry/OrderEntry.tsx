import { useState } from "react";

export interface FakeOrder {
  id: string;
  symbol: string;
  side: "buy" | "sell";
  orderType: "market" | "limit";
  price: string;
  size: string;
}

interface OrderEntryProps {
  symbol: string;
  onOrderFilled: (order: FakeOrder) => void;
}

export function OrderEntry({ symbol, onOrderFilled }: OrderEntryProps) {
  const [side, setSide] = useState<"buy" | "sell">("buy");
  const [orderType, setOrderType] = useState<"market" | "limit">("market");
  const [price, setPrice] = useState("");
  const [size, setSize] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!size) return;

    // TEMPORARY fake fill -- see file header. Real order placement goes
    // through useControlSocket -> place_order -> awaits order_ack/fill.
    const fakeOrder: FakeOrder = {
      id: `fake-${Date.now()}`,
      symbol,
      side,
      orderType,
      price: orderType === "limit" ? price : "market",
      size,
    };
    onOrderFilled(fakeOrder);
    setSize("");
    setPrice("");
  }

  return (
    <form onSubmit={handleSubmit} style={{ background: "#1e222d", padding: 16, borderRadius: 6, minWidth: 240 }}>
      <h3 style={{ marginTop: 0, fontSize: 14 }}>Order Entry ({symbol})</h3>

      <div style={{ marginBottom: 8 }}>
        <button type="button" onClick={() => setSide("buy")} style={{ background: side === "buy" ? "#26a69a" : "#333", color: "#fff", border: "none", padding: "6px 12px", marginRight: 4 }}>
          Buy
        </button>
        <button type="button" onClick={() => setSide("sell")} style={{ background: side === "sell" ? "#ef5350" : "#333", color: "#fff", border: "none", padding: "6px 12px" }}>
          Sell
        </button>
      </div>

      <div style={{ marginBottom: 8 }}>
        <select value={orderType} onChange={(e) => setOrderType(e.target.value as "market" | "limit")}>
          <option value="market">Market</option>
          <option value="limit">Limit</option>
        </select>
      </div>

      {orderType === "limit" && (
        <div style={{ marginBottom: 8 }}>
          <input
            type="text"
            placeholder="Price"
            value={price}
            onChange={(e) => setPrice(e.target.value)}
            style={{ width: "100%", boxSizing: "border-box" }}
          />
        </div>
      )}

      <div style={{ marginBottom: 8 }}>
        <input
          type="text"
          placeholder="Size"
          value={size}
          onChange={(e) => setSize(e.target.value)}
          style={{ width: "100%", boxSizing: "border-box" }}
        />
      </div>

      <button type="submit" style={{ width: "100%", padding: 8, background: "#26a69a", color: "#fff", border: "none", borderRadius: 4, cursor: "pointer" }}>
        Place Order (fake fill)
      </button>
    </form>
  );
}
