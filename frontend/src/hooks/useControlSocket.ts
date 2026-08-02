import { useCallback, useEffect, useRef, useState } from "react";
import type {
  CancelOrderMessage,
  ControlMessage,
  FillSnapshot,
  OrderAckMessage,
  OrderSnapshot,
  PlaceOrderMessage,
  PositionSnapshot,
} from "../types/control";

const CONTROL_WS_URL = "ws://localhost:8000/ws/control";

function closeSafely(ws: WebSocket | null) {
  if (!ws) return;
  if (ws.readyState === WebSocket.CONNECTING) {
    ws.addEventListener("open", () => ws.close());
  } else {
    ws.close();
  }
}

interface PlaceOrderParams {
  symbol: string;
  side: "buy" | "sell";
  orderType: "market" | "limit";
  price: string;
  size: string;
}

function upsertOrder(orders: OrderSnapshot[], order: OrderSnapshot): OrderSnapshot[] {
  const idx = orders.findIndex((o) => o.orderId === order.orderId);
  if (idx === -1) return [order, ...orders];
  const next = orders.slice();
  next[idx] = order;
  return next;
}

export function useControlSocket() {
  const wsRef = useRef<WebSocket | null>(null);
  const [orders, setOrders] = useState<OrderSnapshot[]>([]);
  const [positions, setPositions] = useState<PositionSnapshot[]>([]);
  const [fills, setFills] = useState<FillSnapshot[]>([]);
  const [lastAck, setLastAck] = useState<OrderAckMessage | null>(null);
  // fill_event doesn't carry symbol/side (only orderId/price/size/ts) --
  // this mirrors the latest `orders` for the onmessage handler below
  // (created once, in an effect with `[]` deps) to look those up from,
  // the same ref-mirroring pattern useBookSocket/useCandleSocket already
  // use to keep a stable callback's closure from going stale.
  const ordersRef = useRef<OrderSnapshot[]>(orders);
  ordersRef.current = orders;

  useEffect(() => {
    let ws: WebSocket | null = null;
    let stopped = false;
    let reconnectAttempt = 0;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    function connect() {
      if (stopped) return;
      ws = new WebSocket(CONTROL_WS_URL);
      wsRef.current = ws;

      ws.onopen = () => {
        reconnectAttempt = 0;
      };

      ws.onmessage = (event) => {
        let msg: ControlMessage;
        try {
          msg = JSON.parse(event.data);
        } catch {
          return; // malformed message, safe to ignore
        }

        switch (msg.type) {
          case "state_snapshot":
            setOrders(msg.orders);
            setPositions(msg.positions);
            setFills(msg.fills);
            break;
          case "order_update":
            // Upsert by orderId -- this single rule handles every case: a
            // brand-new order appearing for the first time, a status
            // change on one already known, a cancellation, or a fill.
            setOrders((prev) => upsertOrder(prev, msg.order));
            break;
          case "positions_update":
            setPositions(msg.positions);
            break;
          case "fill_event": {
            const order = ordersRef.current.find((o) => o.orderId === msg.orderId);
            // Don't know this order yet (shouldn't happen in practice --
            // its own order_update always precedes any fill for it) --
            // skip rather than show a fill with blank/wrong symbol/side;
            // the next state_snapshot will include it correctly regardless.
            if (!order) break;
            const fill: FillSnapshot = {
              orderId: msg.orderId,
              symbol: order.symbol,
              side: order.side,
              priceTicks: msg.priceTicks,
              sizeTicks: msg.sizeTicks,
              ts: msg.ts,
            };
            setFills((prev) => [...prev, fill]);
            break;
          }
          case "order_ack":
            setLastAck(msg);
            break;
        }
      };

      ws.onclose = () => {
        // Only clear wsRef if it's still pointing at *this* socket -- a
        // stale/orphaned socket's close event can fire well after a newer
        // one has already taken over (its close() call happens
        // synchronously in cleanup, but the resulting close event doesn't
        // fire until later), and an unconditional null here would wipe out
        // a perfectly good, currently-active connection out from under
        // placeOrder/cancelOrder.
        if (wsRef.current === ws) wsRef.current = null;
        if (stopped) return;
        const delay = Math.min(1000 * 2 ** reconnectAttempt, 15000);
        reconnectAttempt += 1;
        reconnectTimer = setTimeout(connect, delay);
      };

      ws.onerror = () => {
        ws?.close();
      };
    }

    connect();

    return () => {
      stopped = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      closeSafely(ws);
      if (wsRef.current === ws) wsRef.current = null;
    };
  }, []);

  // Returns null when the socket isn't open (mid-reconnect) instead of a
  // clientRequestId that will never resolve -- a caller that set a pending
  // state keyed on that id would otherwise get stuck forever, since no ack
  // is ever coming for a message that was never actually sent.
  const placeOrder = useCallback((params: PlaceOrderParams) => {
    if (wsRef.current?.readyState !== WebSocket.OPEN) return null;
    const clientRequestId = crypto.randomUUID();
    const message: PlaceOrderMessage = { type: "place_order", clientRequestId, ...params };
    wsRef.current.send(JSON.stringify(message));
    return clientRequestId;
  }, []);

  const cancelOrder = useCallback((orderId: number) => {
    if (wsRef.current?.readyState !== WebSocket.OPEN) return null;
    const clientRequestId = crypto.randomUUID();
    const message: CancelOrderMessage = { type: "cancel_order", orderId, clientRequestId };
    wsRef.current.send(JSON.stringify(message));
    return clientRequestId;
  }, []);

  return { orders, positions, fills, placeOrder, cancelOrder, lastAck };
}
