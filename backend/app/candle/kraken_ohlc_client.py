from __future__ import annotations

import asyncio
import json
import logging
from datetime import datetime, timezone
from typing import Callable

import websockets

from app.config import KRAKEN_OHLC_WS_URL

logger = logging.getLogger("candle.kraken_ohlc_client")

SubKey = tuple[str, int]  # (symbol, interval_minutes)
CandleCallback = Callable[[str, int, dict], None]  # (symbol, interval_minutes, candle)


def _to_unix_seconds(iso_ts: str | None) -> int:
    if not iso_ts:
        return 0
    try:
        dt = datetime.fromisoformat(iso_ts.replace("Z", "+00:00"))
        return int(dt.astimezone(timezone.utc).timestamp())
    except ValueError:
        return 0


def format_interval(minutes: int) -> str:
    if minutes < 60:
        return f"{minutes}m"
    if minutes < 1440:
        return f"{minutes // 60}h"
    return f"{minutes // 1440}d"


class KrakenOhlcClient:
    def __init__(self, on_candle: CandleCallback) -> None:
        self._on_candle = on_candle
        self._subscribed: set[SubKey] = set()
        self._ws: websockets.WebSocketClientProtocol | None = None
        self._stop = False
        self._last_interval_begin: dict[SubKey, str] = {}
        self._last_candle: dict[SubKey, dict] = {}

    async def run(self) -> None:
        backoff = 1
        while not self._stop:
            try:
                async with websockets.connect(KRAKEN_OHLC_WS_URL) as ws:
                    self._ws = ws
                    logger.info("candle.kraken connected")
                    backoff = 1
                    for symbol, interval_minutes in list(self._subscribed):
                        await self._send_subscribe(symbol, interval_minutes)
                    async for raw in ws:
                        self._handle_message(raw)
            except asyncio.CancelledError:
                raise
            except Exception:
                logger.exception("candle.kraken connection error, reconnecting")
            finally:
                self._ws = None
            if self._stop:
                break
            await asyncio.sleep(backoff)
            backoff = min(backoff * 2, 64)

    async def stop(self) -> None:
        self._stop = True
        if self._ws is not None:
            await self._ws.close()

    async def subscribe(self, symbol: str, interval_minutes: int) -> None:
        key = (symbol, interval_minutes)
        self._subscribed.add(key)
        if self._ws is not None:
            await self._send_subscribe(symbol, interval_minutes)

    async def unsubscribe(self, symbol: str, interval_minutes: int) -> None:
        key = (symbol, interval_minutes)
        self._subscribed.discard(key)
        self._last_interval_begin.pop(key, None)
        self._last_candle.pop(key, None)
        if self._ws is not None:
            await self._send_unsubscribe(symbol, interval_minutes)

    async def _send_subscribe(self, symbol: str, interval_minutes: int) -> None:
        assert self._ws is not None
        await self._ws.send(json.dumps({
            "method": "subscribe",
            "params": {
                "channel": "ohlc",
                "symbol": [symbol],
                "interval": interval_minutes,
            },
        }))
        logger.info("candle.kraken subscribed symbol=%s interval=%s", symbol, interval_minutes)

    async def _send_unsubscribe(self, symbol: str, interval_minutes: int) -> None:
        assert self._ws is not None
        await self._ws.send(json.dumps({
            "method": "unsubscribe",
            "params": {
                "channel": "ohlc",
                "symbol": [symbol],
                "interval": interval_minutes,
            },
        }))
        logger.info("candle.kraken unsubscribed symbol=%s interval=%s", symbol, interval_minutes)

    def _handle_message(self, raw: str) -> None:
        try:
            msg = json.loads(raw)
        except json.JSONDecodeError:
            return
        if not isinstance(msg, dict) or msg.get("channel") != "ohlc":
            return
        for entry in msg.get("data", []):
            self._handle_candle_entry(entry)

    def _handle_candle_entry(self, entry: dict) -> None:
        symbol = entry.get("symbol")
        interval_minutes = entry.get("interval")
        if not symbol or interval_minutes is None:
            return
        key = (symbol, interval_minutes)

        interval_begin = entry.get("interval_begin")
        prev_begin = self._last_interval_begin.get(key)
        if prev_begin is not None and interval_begin != prev_begin:
            prev_candle = self._last_candle.get(key)
            if prev_candle is not None:
                self._on_candle(symbol, interval_minutes, {**prev_candle, "closed": True})

        candle = {
            "symbol": symbol,
            "interval": format_interval(interval_minutes),
            "time": _to_unix_seconds(interval_begin),
            "open": entry.get("open"),
            "high": entry.get("high"),
            "low": entry.get("low"),
            "close": entry.get("close"),
            "volume": entry.get("volume"),
            "closed": False,
        }
        self._last_interval_begin[key] = interval_begin
        self._last_candle[key] = candle
        self._on_candle(symbol, interval_minutes, candle)
