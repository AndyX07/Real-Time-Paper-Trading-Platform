import { useEffect, useState } from "react";
import type { OrderAckMessage } from "../../types/control";

interface PlaceOrderParams {
  symbol: string;
  side: "buy" | "sell";
  orderType: "market" | "limit";
  price: string;
  size: string;
}

interface OrderEntryProps {
  symbol: string;
  placeOrder: (params: PlaceOrderParams) => string | null;
  lastAck: OrderAckMessage | null;
}

export function OrderEntry({ symbol, placeOrder, lastAck }: OrderEntryProps) {
  const [side, setSide] = useState<"buy" | "sell">("buy");
  const [orderType, setOrderType] = useState<"market" | "limit">("limit");
  const [price, setPrice] = useState("");
  const [size, setSize] = useState("");
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // order_ack is broadcast to every connected client, so this only reacts
  // once lastAck.clientRequestId matches the request this form itself
  // made -- the accompanying order_update (handled by useControlSocket)
  // is what actually populates the orders list, for this tab and every
  // other one; this effect only resolves this form's own pending-submit UI.
  useEffect(() => {
    if (!pendingId || !lastAck || lastAck.clientRequestId !== pendingId) return;
    setPendingId(null);
    if (lastAck.status === "rejected") {
      setError(lastAck.reason ?? "order rejected");
    } else {
      setError(null);
      setSize("");
      setPrice("");
    }
  }, [lastAck, pendingId]);

  const total = orderType === "limit" && price && size ? Number(price) * Number(size) : null;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!size || pendingId) return;
    const clientRequestId = placeOrder({ symbol, side, orderType, price, size });
    if (clientRequestId == null) {
      // Not connected right now -- fail immediately rather than entering a
      // pending state that would never resolve (no message was actually
      // sent, so no ack is ever coming).
      setError("not connected -- try again in a moment");
      return;
    }
    setError(null);
    setPendingId(clientRequestId);
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
          disabled={pendingId != null}
          className={`mt-2 rounded-lg py-2.5 text-sm font-semibold text-white transition-colors disabled:opacity-50 ${
            side === "buy" ? "bg-buy hover:opacity-90" : "bg-sell hover:opacity-90"
          }`}
        >
          {pendingId != null ? "Placing..." : `${side === "buy" ? "Buy" : "Sell"} ${base}`}
        </button>

        {error && <p className="text-center text-[11px] text-sell">{error}</p>}
      </div>
    </form>
  );
}
