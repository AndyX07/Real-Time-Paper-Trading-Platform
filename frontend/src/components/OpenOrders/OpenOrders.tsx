import type { FakeOrder } from "../OrderEntry/OrderEntry";

interface OpenOrdersProps {
  orders: FakeOrder[];
}

export function OpenOrders({ orders }: OpenOrdersProps) {
  return (
    <div style={{ background: "#1e222d", padding: 16, borderRadius: 6, flex: 1 }}>
      <h3 style={{ marginTop: 0, fontSize: 14 }}>Orders (fake fills)</h3>
      {orders.length === 0 ? (
        <p style={{ color: "#888", fontSize: 13 }}>No orders yet.</p>
      ) : (
        <table style={{ width: "100%", fontSize: 13, borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ textAlign: "left", color: "#888" }}>
              <th>Symbol</th>
              <th>Side</th>
              <th>Type</th>
              <th>Price</th>
              <th>Size</th>
            </tr>
          </thead>
          <tbody>
            {orders.map((o) => (
              <tr key={o.id}>
                <td>{o.symbol}</td>
                <td style={{ color: o.side === "buy" ? "#26a69a" : "#ef5350" }}>{o.side}</td>
                <td>{o.orderType}</td>
                <td>{o.price}</td>
                <td>{o.size}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
