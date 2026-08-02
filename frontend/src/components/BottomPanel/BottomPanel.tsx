import { useState } from "react";
import type { FillSnapshot, OrderAckMessage, OrderSnapshot, PositionSnapshot } from "../../types/control";
import { OpenOrders } from "../OpenOrders/OpenOrders";
import { FillHistory } from "../FillHistory/FillHistory";
import { PnlDisplay } from "../PnlDisplay/PnlDisplay";

const TABS = ["Positions", "Open Orders", "Closed Orders", "Trades"] as const;

const OPEN_STATUSES = new Set(["pending", "open", "partially_filled"]);

interface BottomPanelProps {
  orders: OrderSnapshot[];
  positions: PositionSnapshot[];
  fills: FillSnapshot[];
  cancelOrder: (orderId: number) => string | null;
  lastAck: OrderAckMessage | null;
  selectedSymbol: string;
  midPrice: number | null;
}

export function BottomPanel({
  orders,
  positions,
  fills,
  cancelOrder,
  lastAck,
  selectedSymbol,
  midPrice,
}: BottomPanelProps) {
  const [active, setActive] = useState<(typeof TABS)[number]>("Open Orders");

  const openOrders = orders.filter((o) => OPEN_STATUSES.has(o.status));
  const closedOrders = orders.filter((o) => !OPEN_STATUSES.has(o.status));

  return (
    <div className="flex h-64 flex-col bg-panel">
      <div className="flex gap-6 overflow-x-auto overflow-y-hidden border-b border-border px-3 text-sm font-medium">
        {TABS.map((tab) => (
          <button
            key={tab}
            onClick={() => setActive(tab)}
            className={`-mb-px shrink-0 border-b-2 py-2.5 transition-colors ${
              active === tab
                ? "border-text-primary text-text-primary"
                : "border-transparent text-text-muted hover:text-text-primary"
            }`}
          >
            {tab}
          </button>
        ))}
      </div>
      <div className="flex-1 overflow-auto">
        {active === "Open Orders" && <OpenOrders orders={openOrders} cancelOrder={cancelOrder} lastAck={lastAck} />}
        {active === "Closed Orders" && <OpenOrders orders={closedOrders} />}
        {active === "Trades" && <FillHistory fills={fills} />}
        {active === "Positions" && (
          <PnlDisplay positions={positions} selectedSymbol={selectedSymbol} midPrice={midPrice} />
        )}
      </div>
    </div>
  );
}
