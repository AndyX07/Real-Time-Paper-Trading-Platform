import { useEffect, useRef } from "react";
import type { BookMessage } from "../types/book";

const BOOK_WS_URL = "ws://localhost:8000/ws/book";

// Aborting a still-CONNECTING socket can leave the underlying handshake in
// a state that interferes with an immediate reconnect to the same
// host:port on some platforms -- exactly what React StrictMode's
// synchronous mount->cleanup->remount does in dev (observed: it broke the
// very first connection every time until this was fixed). Deferring the
// close until the socket actually finishes opening avoids ever aborting a
// handshake mid-flight.
function closeSafely(ws: WebSocket | null) {
  if (!ws) return;
  if (ws.readyState === WebSocket.CONNECTING) {
    ws.addEventListener("open", () => ws.close());
  } else {
    ws.close();
  }
}

export function useBookSocket(symbol: string, onMessage: (msg: BookMessage) => void) {
  const onMessageRef = useRef(onMessage);
  onMessageRef.current = onMessage;

  useEffect(() => {
    let ws: WebSocket | null = null;
    let stopped = false;
    let reconnectAttempt = 0;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    function connect() {
      if (stopped) return;
      ws = new WebSocket(BOOK_WS_URL);

      ws.onopen = () => {
        reconnectAttempt = 0;
        ws?.send(JSON.stringify({ type: "subscribe_book", symbol }));
      };

      ws.onmessage = (event) => {
        let msg: BookMessage;
        try {
          msg = JSON.parse(event.data);
        } catch {
          return; // malformed message, safe to ignore
        }
        if (msg.type !== "book_delta" && msg.type !== "book_snapshot") return;
        onMessageRef.current(msg);
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
        ws.send(JSON.stringify({ type: "unsubscribe_book", symbol }));
      }
      closeSafely(ws);
    };
  }, [symbol]);
}
