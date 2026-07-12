from __future__ import annotations

import httpx
from app.config import KRAKEN_REST_OHLC_URL


async def fetch_ohlc_history(symbol: str, interval_minutes: int, count: int = 720) -> list[dict]:
    async with httpx.AsyncClient(timeout=10.0) as client:
        resp = await client.get(
            KRAKEN_REST_OHLC_URL,
            params={"pair": symbol, "interval": interval_minutes},
        )
        resp.raise_for_status()
        payload = resp.json()

    if payload.get("error"):
        raise RuntimeError(f"Kraken OHLC history error: {payload['error']}")

    result = payload.get("result", {})
    bars = result.get(symbol)
    if bars is None:
        # Kraken echoed back a different key than requested with (can
        # happen for legacy-aliased pairs) -- fall back to whichever
        # non-"last" key is present rather than silently returning nothing.
        candidates = [k for k in result.keys() if k != "last"]
        bars = result.get(candidates[0], []) if candidates else []

    candles = [
        {
            "time": int(bar[0]),
            "open": float(bar[1]),
            "high": float(bar[2]),
            "low": float(bar[3]),
            "close": float(bar[4]),
        }
        for bar in bars
    ]
    return candles[-count:]
