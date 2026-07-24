export interface PriceLevelMessage {
  priceTicks: number;
  sizeTicks: number;
}

export interface BookDeltaMessage {
  type: "book_delta";
  symbol: string;
  seq: number;
  side: "bid" | "ask";
  priceTicks: number;
  sizeTicks: number;
}

export interface BookSnapshotMessage {
  type: "book_snapshot";
  symbol: string;
  seq: number;
  bids: PriceLevelMessage[];
  asks: PriceLevelMessage[];
}

export type BookMessage = BookDeltaMessage | BookSnapshotMessage;
