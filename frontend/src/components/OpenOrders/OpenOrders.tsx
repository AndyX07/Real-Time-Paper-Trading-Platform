import type { FakeOrder } from "../OrderEntry/OrderEntry";

interface OpenOrdersProps {
  orders: FakeOrder[];
}

export function OpenOrders({ orders }: OpenOrdersProps) {
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
        </tr>
      </thead>
      <tbody>
        {orders.map((o) => (
          <tr key={o.id} className="border-t border-border">
            <td className="px-3 py-2">{o.symbol}</td>
            <td className={`px-3 py-2 capitalize ${o.side === "buy" ? "text-buy" : "text-sell"}`}>{o.side}</td>
            <td className="px-3 py-2 capitalize">{o.orderType}</td>
            <td className="px-3 py-2">{o.price}</td>
            <td className="px-3 py-2">{o.size}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
