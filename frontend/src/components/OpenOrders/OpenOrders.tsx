import { useEffect, useState } from "react";
import type { OrderAckMessage, OrderSnapshot } from "../../types/control";

// Same 1e10 scale duplicated locally per the codebase's existing
// convention (see ladderLevels.ts) rather than imported from a shared
// helper.
const PRICE_SCALE = 1e10;
const QUANTITY_SCALE = 1e10;

function formatOptionalTicks(ticks: number | undefined, scale: number): string {
  return ticks == null ? "market" : (ticks / scale).toString();
}

interface OpenOrdersProps {
  orders: OrderSnapshot[];
  // Omitted entirely for a read-only table (e.g. Closed Orders) -- no
  // Cancel column is rendered at all in that case.
  cancelOrder?: (orderId: number) => string | null;
  lastAck?: OrderAckMessage | null;
}

export function OpenOrders({ orders, cancelOrder, lastAck }: OpenOrdersProps) {
  // orderId -> the clientRequestId of that row's own in-flight cancel, so
  // one row's button disables independently of every other row's.
  const [pendingCancels, setPendingCancels] = useState<Record<number, string>>({});

  useEffect(() => {
    if (!lastAck) return;
    setPendingCancels((prev) => {
      const next = { ...prev };
      for (const [orderId, clientRequestId] of Object.entries(next)) {
        if (clientRequestId === lastAck.clientRequestId) {
          delete next[Number(orderId)];
        }
      }
      return next;
    });
  }, [lastAck]);

  function handleCancel(orderId: number) {
    if (!cancelOrder) return;
    const clientRequestId = cancelOrder(orderId);
    // Not connected right now -- no message was sent, so no ack is ever
    // coming; leave the button enabled rather than getting stuck on
    // "Canceling..." forever.
    if (clientRequestId == null) return;
    setPendingCancels((prev) => ({ ...prev, [orderId]: clientRequestId }));
  }

  if (orders.length === 0) {
    return <div className="flex h-full items-center justify-center text-sm text-text-muted">No orders yet.</div>;
  }

  return (
    <table className="w-full text-left text-xs">
      <thead>
        <tr className="text-text-muted">
          <th className="px-3 py-2 font-medium">Symbol</th>
          <th className="px-3 py-2 font-medium">Side</th>
          <th className="px-3 py-2 font-medium">Type</th>
          <th className="px-3 py-2 font-medium">Price</th>
          <th className="px-3 py-2 font-medium">Size</th>
          <th className="px-3 py-2 font-medium">Status</th>
          {cancelOrder && <th className="px-3 py-2 font-medium" />}
        </tr>
      </thead>
      <tbody>
        {orders.map((o) => (
          <tr key={o.orderId} className="border-t border-border">
            <td className="px-3 py-2">{o.symbol}</td>
            <td className={`px-3 py-2 capitalize ${o.side === "buy" ? "text-buy" : "text-sell"}`}>{o.side}</td>
            <td className="px-3 py-2 capitalize">{o.orderType}</td>
            <td className="px-3 py-2">{formatOptionalTicks(o.priceTicks, PRICE_SCALE)}</td>
            <td className="px-3 py-2">{(o.sizeTicks / QUANTITY_SCALE).toString()}</td>
            <td className="px-3 py-2 capitalize">{o.status.replace("_", " ")}</td>
            {cancelOrder && (
              <td className="px-3 py-2">
                <button
                  type="button"
                  onClick={() => handleCancel(o.orderId)}
                  disabled={pendingCancels[o.orderId] != null}
                  className="rounded px-2 py-1 text-[11px] font-medium text-text-muted ring-1 ring-border hover:text-text-primary disabled:opacity-50"
                >
                  {pendingCancels[o.orderId] != null ? "Canceling..." : "Cancel"}
                </button>
              </td>
            )}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
