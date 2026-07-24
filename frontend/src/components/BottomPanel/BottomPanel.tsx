import { useState } from "react";
import type { FakeOrder } from "../OrderEntry/OrderEntry";
import { OpenOrders } from "../OpenOrders/OpenOrders";

const TABS = ["Balances", "Positions", "Open Orders", "Conditional Orders", "Portfolio", "Closed Orders", "Trades"] as const;

interface BottomPanelProps {
  orders: FakeOrder[];
}

// Only "Open Orders" has real (fake-fill) data behind it right now --
// FillHistory/PnlDisplay aren't implemented yet, and there's no
// balances/positions/portfolio backend at all, so every other tab is a
// placeholder rather than something wired to invented data.
export function BottomPanel({ orders }: BottomPanelProps) {
  const [active, setActive] = useState<(typeof TABS)[number]>("Open Orders");

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
        {active === "Open Orders" ? (
          <OpenOrders orders={orders} />
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-text-muted">Nothing here yet.</div>
        )}
      </div>
    </div>
  );
}
