export interface PlaceOrderMessage {
  type: "place_order";
  symbol: string;
  side: "buy" | "sell";
  orderType: "market" | "limit";
  price: string; // decimal string; ignored for market
  size: string; // decimal string
  clientRequestId: string;
}

export interface CancelOrderMessage {
  type: "cancel_order";
  orderId: number;
  clientRequestId: string;
}

export interface OrderAckMessage {
  type: "order_ack";
  clientRequestId: string;
  orderId: number;
  status: "accepted" | "rejected";
  reason?: string;
}

export interface FillEventMessage {
  type: "fill_event";
  orderId: number;
  priceTicks: number;
  sizeTicks: number;
  ts: number;
}

export interface OrderUpdateMessage {
  type: "order_update";
  order: OrderSnapshot;
}

export interface OrderSnapshot {
  orderId: number;
  engineOrderId?: number;
  symbol: string;
  side: "buy" | "sell";
  orderType: "market" | "limit";
  priceTicks?: number;
  sizeTicks: number;
  status: string;
  cancelReason?: string;
  rejectReason?: string;
  createdAt: number;
  updatedAt: number;
}

export interface PositionSnapshot {
  symbol: string;
  netSizeTicks: number;
  avgCostTicks: number;
  realizedPnlTicks: number;
}

export interface FillSnapshot {
  orderId: number;
  symbol: string;
  side: "buy" | "sell";
  priceTicks: number;
  sizeTicks: number;
  ts: number;
}

export interface StateSnapshotMessage {
  type: "state_snapshot";
  orders: OrderSnapshot[];
  positions: PositionSnapshot[];
  fills: FillSnapshot[];
}

export interface PositionsUpdateMessage {
  type: "positions_update";
  positions: PositionSnapshot[];
}

export type ControlMessage =
  | OrderAckMessage
  | FillEventMessage
  | OrderUpdateMessage
  | StateSnapshotMessage
  | PositionsUpdateMessage;
