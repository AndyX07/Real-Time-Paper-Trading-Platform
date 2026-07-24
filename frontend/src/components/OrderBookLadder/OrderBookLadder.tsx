import { useEffect, useRef, useState } from "react";
import { useBookSocket } from "../../hooks/useBookSocket";
import { BookStore } from "../../state/bookStore";
import { buildLadderLevels, type LadderLevel } from "./ladderLevels";

interface OrderBookLadderProps {
  symbol: string;
  priceDecimals: number;
  quantityDecimals: number;
}

const LEVELS_PER_SIDE = 12;

interface LadderSnapshot {
  asks: LadderLevel[]; // best (lowest) first
  bids: LadderLevel[]; // best (highest) first
}

const EMPTY: LadderSnapshot = { asks: [], bids: [] };

export function OrderBookLadder({ symbol, priceDecimals, quantityDecimals }: OrderBookLadderProps) {
  const storeRef = useRef(new BookStore());
  const [snapshot, setSnapshot] = useState<LadderSnapshot>(EMPTY);

  // Fresh store per symbol -- a stale book from the previous symbol must
  // never bleed into the next one's first frame.
  useEffect(() => {
    storeRef.current = new BookStore();
    setSnapshot(EMPTY);
  }, [symbol]);

  useBookSocket(symbol, (msg) => {
    if (msg.type === "book_snapshot") {
      storeRef.current.applySnapshot(msg);
    } else {
      storeRef.current.applyDelta(msg);
    }
  });

  // requestAnimationFrame render loop, deliberately decoupled from the
  // WebSocket message rate (ARCHITECTURE.md 4.7) -- reads whatever's
  // currently in storeRef, never the message stream directly.
  useEffect(() => {
    let rafId: number;
    function frame() {
      const store = storeRef.current;
      const asks = buildLadderLevels(store.getAsks(), (a, b) => a - b, LEVELS_PER_SIDE);
      const bids = buildLadderLevels(store.getBids(), (a, b) => b - a, LEVELS_PER_SIDE);
      setSnapshot({ asks, bids });
      rafId = requestAnimationFrame(frame);
    }
    rafId = requestAnimationFrame(frame);
    return () => cancelAnimationFrame(rafId);
  }, []);

  const { asks, bids } = snapshot;
  const bestBid = bids[0]?.price;
  const bestAsk = asks[0]?.price;
  const spread = bestBid != null && bestAsk != null ? bestAsk - bestBid : null;
  const spreadPct = spread != null && bestAsk ? (spread / bestAsk) * 100 : null;
  const maxCum = Math.max(asks[asks.length - 1]?.cumSize ?? 0, bids[bids.length - 1]?.cumSize ?? 0) || 1;
  const isEmpty = asks.length === 0 && bids.length === 0;

  return (
    <div className="flex h-full flex-col bg-panel text-text-primary">
      <div className="border-b border-border px-3 py-2 text-sm font-semibold">Order Book</div>
      <div className="flex justify-between px-3 py-1.5 text-[11px] font-medium tracking-wide text-text-muted">
        <span>PRICE</span>
        <span>QUANTITY</span>
      </div>

      {isEmpty ? (
        <div className="flex flex-1 items-center justify-center text-sm text-text-muted">
          waiting for book data...
        </div>
      ) : (
        <>
          {/* Asks: best (lowest) ask closest to the spread, worst at the top. */}
          <div className="flex flex-col-reverse overflow-hidden">
            {asks.map((level) => (
              <LadderRow
                key={level.price}
                level={level}
                maxCum={maxCum}
                side="ask"
                priceDecimals={priceDecimals}
                quantityDecimals={quantityDecimals}
              />
            ))}
          </div>

          <div className="flex justify-between border-y border-border bg-panel-alt px-3 py-1.5 text-xs tabular-nums">
            <span className="text-text-muted">Spread</span>
            <span>
              {spread != null ? spread.toFixed(priceDecimals) : "--"}
              {spreadPct != null && <span className="text-text-muted"> ({spreadPct.toFixed(4)}%)</span>}
            </span>
          </div>

          {/* Bids: best (highest) bid closest to the spread, worst at the bottom. */}
          <div className="flex flex-col overflow-hidden">
            {bids.map((level) => (
              <LadderRow
                key={level.price}
                level={level}
                maxCum={maxCum}
                side="bid"
                priceDecimals={priceDecimals}
                quantityDecimals={quantityDecimals}
              />
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function LadderRow({
  level,
  maxCum,
  side,
  priceDecimals,
  quantityDecimals,
}: {
  level: LadderLevel;
  maxCum: number;
  side: "bid" | "ask";
  priceDecimals: number;
  quantityDecimals: number;
}) {
  const pct = Math.min(100, (level.cumSize / maxCum) * 100);
  const color = side === "bid" ? "text-buy" : "text-sell";
  const barColor = side === "bid" ? "bg-buy/15" : "bg-sell/15";

  return (
    <div className="relative flex justify-between px-3 py-0.5 text-xs tabular-nums hover:bg-white/[0.04]">
      <div className={`absolute inset-y-0 left-0 ${barColor}`} style={{ width: `${pct}%` }} />
      <span className={`relative z-10 font-medium ${color}`}>
        {level.price.toLocaleString(undefined, { minimumFractionDigits: priceDecimals, maximumFractionDigits: priceDecimals })}
      </span>
      <span className="relative z-10">{level.size.toFixed(quantityDecimals)}</span>
    </div>
  );
}
