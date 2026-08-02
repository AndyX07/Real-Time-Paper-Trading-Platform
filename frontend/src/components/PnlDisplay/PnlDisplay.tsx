import type { PositionSnapshot } from "../../types/control";

// Same 1e10 scale duplicated locally per the codebase's existing
// convention (see ladderLevels.ts) rather than imported from a shared
// helper.
const PRICE_SCALE = 1e10;
const QUANTITY_SCALE = 1e10;

interface PnlDisplayProps {
  positions: PositionSnapshot[];
  // Live mid-price for selectedSymbol only -- book data only exists for
  // whichever single symbol's ladder is currently mounted, so unrealized
  // P&L can only ever be computed for that one symbol at a time.
  selectedSymbol: string;
  midPrice: number | null;
}

export function PnlDisplay({ positions, selectedSymbol, midPrice }: PnlDisplayProps) {
  if (positions.length === 0) {
    return <div className="flex h-full items-center justify-center text-sm text-text-muted">No positions yet.</div>;
  }

  return (
    <table className="w-full text-left text-xs">
      <thead>
        <tr className="text-text-muted">
          <th className="px-3 py-2 font-medium">Symbol</th>
          <th className="px-3 py-2 font-medium">Net Size</th>
          <th className="px-3 py-2 font-medium">Avg Cost</th>
          <th className="px-3 py-2 font-medium">Realized P&L</th>
          <th className="px-3 py-2 font-medium">Unrealized P&L</th>
        </tr>
      </thead>
      <tbody>
        {positions.map((p) => {
          const netSize = p.netSizeTicks / QUANTITY_SCALE;
          const avgCost = p.avgCostTicks / PRICE_SCALE;
          const realized = p.realizedPnlTicks / PRICE_SCALE;
          const isSelected = p.symbol === selectedSymbol;
          const unrealized = isSelected && netSize !== 0 && midPrice != null ? netSize * (midPrice - avgCost) : null;

          return (
            <tr key={p.symbol} className="border-t border-border">
              <td className="px-3 py-2">{p.symbol}</td>
              <td className={`px-3 py-2 ${netSize > 0 ? "text-buy" : netSize < 0 ? "text-sell" : ""}`}>
                {netSize.toString()}
              </td>
              <td className="px-3 py-2">{netSize !== 0 ? avgCost.toFixed(2) : "--"}</td>
              <td className={`px-3 py-2 ${realized > 0 ? "text-buy" : realized < 0 ? "text-sell" : ""}`}>
                {realized.toFixed(2)}
              </td>
              <td className="px-3 py-2">
                {unrealized != null ? (
                  <span className={unrealized >= 0 ? "text-buy" : "text-sell"}>{unrealized.toFixed(2)}</span>
                ) : (
                  <span className="text-text-muted">{netSize === 0 ? "--" : "select symbol"}</span>
                )}
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
