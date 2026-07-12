from __future__ import annotations

import asyncio
import logging

from fastapi import APIRouter, Query, WebSocket, WebSocketDisconnect
from pydantic import ValidationError

from app.candle.kraken_ohlc_client import KrakenOhlcClient
from app.candle.kraken_ohlc_history import fetch_ohlc_history
from app.candle.subscription_manager import CandleSubscriptionManager
from app.schemas.candle import CandleMessage

logger = logging.getLogger("candle.ws_router")

router = APIRouter()
subscription_manager = CandleSubscriptionManager()


def _on_candle(symbol: str, interval_minutes: int, candle: dict) -> None:
    # Called synchronously from KrakenOhlcClient's message-handling path;
    # fan-out to browser clients is async, so each send is scheduled as
    # its own task rather than awaited here.
    try:
        message = CandleMessage(**candle).model_dump()
    except ValidationError:
        logger.warning("candle.ws_router: dropping malformed candle for %s", symbol)
        return
    for client in list(subscription_manager.clients_for(symbol, interval_minutes)):
        asyncio.create_task(_safe_send(client, message))


async def _safe_send(client: WebSocket, message: dict) -> None:
    try:
        await client.send_json(message)
    except Exception:
        logger.warning("candle.ws_router: failed to send to a client, dropping")


kraken_ohlc_client = KrakenOhlcClient(on_candle=_on_candle)


@router.get("/api/candles/history")
async def candles_history(
    symbol: str = Query(...),
    interval: int = Query(1),
    count: int = Query(720, le=720),
) -> list[dict]:
    try:
        return await fetch_ohlc_history(symbol, interval, count)
    except Exception:
        logger.exception("candle.ws_router: backfill fetch failed for %s@%s", symbol, interval)
        return []


@router.websocket("/ws/candles")
async def candles_ws(websocket: WebSocket) -> None:
    await websocket.accept()
    try:
        while True:
            msg = await websocket.receive_json()
            msg_type = msg.get("type")
            symbol = msg.get("symbol")
            interval_minutes = msg.get("interval", 1)
            if msg_type == "subscribe_candle" and symbol:
                if subscription_manager.subscribe(symbol, interval_minutes, websocket):
                    await kraken_ohlc_client.subscribe(symbol, interval_minutes)
            elif msg_type == "unsubscribe_candle" and symbol:
                if subscription_manager.unsubscribe(symbol, interval_minutes, websocket):
                    await kraken_ohlc_client.unsubscribe(symbol, interval_minutes)
    except WebSocketDisconnect:
        pass
    finally:
        emptied = subscription_manager.unsubscribe_all(websocket)
        for symbol, interval_minutes in emptied:
            await kraken_ohlc_client.unsubscribe(symbol, interval_minutes)
