import type { BookDeltaMessage, BookSnapshotMessage } from "../types/book";

// Plain Map<priceTicks, sizeTicks> per side, mutated synchronously on every
// book WebSocket message (ARCHITECTURE.md 4.7) -- the frontend-side analogue
// of the backend's ring buffer: decouples producer rate (network) from
// consumer rate (DepthChart's rAF redraw) via a buffer that always holds
// current state, not a queue of every intermediate state. Keyed/valued by
// integer ticks (4.12); conversion to a display price/size happens only in
// depthRenderer.ts at draw time.
export class BookStore {
  private bids = new Map<number, number>();
  private asks = new Map<number, number>();
  private seq: number | null = null;

  applySnapshot(msg: BookSnapshotMessage): void {
    this.bids.clear();
    this.asks.clear();
    for (const level of msg.bids) this.bids.set(level.priceTicks, level.sizeTicks);
    for (const level of msg.asks) this.asks.set(level.priceTicks, level.sizeTicks);
    this.seq = msg.seq;
  }

  applyDelta(msg: BookDeltaMessage): void {
    const side = msg.side === "bid" ? this.bids : this.asks;
    if (msg.sizeTicks === 0) {
      side.delete(msg.priceTicks);
    } else {
      side.set(msg.priceTicks, msg.sizeTicks);
    }
    this.seq = msg.seq;
  }

  getBids(): ReadonlyMap<number, number> {
    return this.bids;
  }

  getAsks(): ReadonlyMap<number, number> {
    return this.asks;
  }

  getSeq(): number | null {
    return this.seq;
  }
}
