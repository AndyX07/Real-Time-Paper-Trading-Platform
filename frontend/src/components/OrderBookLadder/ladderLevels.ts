const PRICE_SCALE = 1e10;
const QUANTITY_SCALE = 1e10;

export interface LadderLevel {
  price: number;
  size: number;
  cumSize: number;
}

// Cumulative summation happens on integer ticks (ARCHITECTURE.md 4.12);
// division into display price/size only happens here, at render time.
export function buildLadderLevels(
  levels: ReadonlyMap<number, number>,
  bestFirst: (a: number, b: number) => number,
  limit: number,
): LadderLevel[] {
  const sorted = Array.from(levels.keys()).sort(bestFirst).slice(0, limit);
  let cumTicks = 0;
  return sorted.map((priceTicks) => {
    const sizeTicks = levels.get(priceTicks)!;
    cumTicks += sizeTicks;
    return {
      price: priceTicks / PRICE_SCALE,
      size: sizeTicks / QUANTITY_SCALE,
      cumSize: cumTicks / QUANTITY_SCALE,
    };
  });
}
