import { useEffect, useRef } from "react";
import type { CandleMessage } from "../types/candle";

const CANDLE_WS_URL = "ws://localhost:8000/ws/candles";

export function useCandleSocket(
  symbol: string,
  intervalMinutes: number,
  onCandle: (candle: CandleMessage) => void,
) {
  const onCandleRef = useRef(onCandle);
  onCandleRef.current = onCandle;

  useEffect(() => {
    let ws: WebSocket | null = null;
    let stopped = false;
    let reconnectAttempt = 0;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    function connect() {
      if (stopped) return;
      ws = new WebSocket(CANDLE_WS_URL);

      ws.onopen = () => {
        reconnectAttempt = 0;
        ws?.send(JSON.stringify({ type: "subscribe_candle", symbol, interval: intervalMinutes }));
      };

      ws.onmessage = (event) => {
        let msg: CandleMessage;
        try {
          msg = JSON.parse(event.data);
        } catch {
          return; // malformed message, safe to ignore
        }
        if (msg.type !== "candle") return;
        // Deliberately outside the try/catch above: if the chart-update
        // callback throws (e.g. lightweight-charts rejecting an
        // out-of-order timestamp), that needs to be visible, not
        // silently swallowed alongside JSON parse errors -- a bare
        // catch here previously hid exactly this class of bug.
        onCandleRef.current(msg);
      };

      ws.onclose = () => {
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
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "unsubscribe_candle", symbol, interval: intervalMinutes }));
      }
      ws?.close();
    };
  }, [symbol, intervalMinutes]);
}
