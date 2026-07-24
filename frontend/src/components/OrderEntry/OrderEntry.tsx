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
  const [orderType, setOrderType] = useState<"market" | "limit">("limit");
  const [price, setPrice] = useState("");
  const [size, setSize] = useState("");

  const total = orderType === "limit" && price && size ? Number(price) * Number(size) : null;

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

  const [base, quote] = symbol.split("/");

  return (
    <form onSubmit={handleSubmit} className="flex h-full flex-col bg-panel text-text-primary">
      <div className="grid grid-cols-2 gap-px bg-border">
        <button
          type="button"
          onClick={() => setSide("buy")}
          className={`py-2.5 text-sm font-semibold transition-colors ${
            side === "buy" ? "bg-buy text-white" : "bg-panel-alt text-text-muted hover:text-text-primary"
          }`}
        >
          Buy
        </button>
        <button
          type="button"
          onClick={() => setSide("sell")}
          className={`py-2.5 text-sm font-semibold transition-colors ${
            side === "sell" ? "bg-sell text-white" : "bg-panel-alt text-text-muted hover:text-text-primary"
          }`}
        >
          Sell
        </button>
      </div>

      <div className="flex gap-4 border-b border-border px-3 pt-2.5 text-sm">
        {(["limit", "market"] as const).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setOrderType(t)}
            className={`-mb-px border-b-2 pb-2 capitalize transition-colors ${
              orderType === t
                ? "border-text-primary text-text-primary"
                : "border-transparent text-text-muted hover:text-text-primary"
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      <div className="flex flex-col gap-3 p-3">
        {orderType === "limit" && (
          <label className="flex flex-col gap-1 text-xs text-text-muted">
            Limit price USD
            <input
              type="text"
              placeholder="0.00"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
              className="rounded-lg bg-panel-alt px-3 py-2.5 text-base font-medium text-text-primary outline-none ring-1 ring-border focus:ring-text-muted"
            />
          </label>
        )}

        <div className="grid grid-cols-2 gap-3">
          <label className="flex flex-col gap-1 text-xs text-text-muted">
            Quantity {base}
            <input
              type="text"
              placeholder="0.00"
              value={size}
              onChange={(e) => setSize(e.target.value)}
              className="rounded-lg bg-panel-alt px-3 py-2.5 text-base font-medium text-text-primary outline-none ring-1 ring-border focus:ring-text-muted"
            />
          </label>

          <div className="flex flex-col gap-1 text-xs text-text-muted">
            Total {quote}
            <div className="flex h-[42px] items-center rounded-lg bg-panel-alt px-3 text-base font-medium text-text-primary ring-1 ring-border">
              {total != null ? `≈ ${total.toLocaleString(undefined, { maximumFractionDigits: 2 })}` : "--"}
            </div>
          </div>
        </div>

        <button
          type="submit"
          className={`mt-2 rounded-lg py-2.5 text-sm font-semibold text-white transition-colors ${
            side === "buy" ? "bg-buy hover:opacity-90" : "bg-sell hover:opacity-90"
          }`}
        >
          {side === "buy" ? "Buy" : "Sell"} {base} (fake fill)
        </button>

        <p className="text-center text-[11px] text-text-muted">
          Paper trading only -- fills are simulated, not routed to the engine yet.
        </p>
      </div>
    </form>
  );
}
