import type { FillSnapshot } from "../../types/control";

// Same 1e10 scale duplicated locally per the codebase's existing
// convention (see ladderLevels.ts) rather than imported from a shared
// helper.
const PRICE_SCALE = 1e10;
const QUANTITY_SCALE = 1e10;

interface FillHistoryProps {
  fills: FillSnapshot[];
}

export function FillHistory({ fills }: FillHistoryProps) {
  if (fills.length === 0) {
    return <div className="flex h-full items-center justify-center text-sm text-text-muted">No fills yet.</div>;
  }

  // ts is nanoseconds since epoch -- newest first.
  const sorted = [...fills].sort((a, b) => b.ts - a.ts);

  return (
    <table className="w-full text-left text-xs">
      <thead>
        <tr className="text-text-muted">
          <th className="px-3 py-2 font-medium">Time</th>
          <th className="px-3 py-2 font-medium">Symbol</th>
          <th className="px-3 py-2 font-medium">Side</th>
          <th className="px-3 py-2 font-medium">Price</th>
          <th className="px-3 py-2 font-medium">Size</th>
        </tr>
      </thead>
      <tbody>
        {sorted.map((f, i) => (
          <tr key={`${f.orderId}-${f.ts}-${i}`} className="border-t border-border">
            <td className="px-3 py-2">{new Date(f.ts / 1e6).toLocaleTimeString()}</td>
            <td className="px-3 py-2">{f.symbol}</td>
            <td className={`px-3 py-2 capitalize ${f.side === "buy" ? "text-buy" : "text-sell"}`}>{f.side}</td>
            <td className="px-3 py-2">{(f.priceTicks / PRICE_SCALE).toString()}</td>
            <td className="px-3 py-2">{(f.sizeTicks / QUANTITY_SCALE).toString()}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
