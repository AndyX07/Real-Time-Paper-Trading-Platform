export interface CandleMessage {
  type: "candle";
  symbol: string;
  interval: string;
  time: number; // unix seconds
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  closed: boolean;
}
