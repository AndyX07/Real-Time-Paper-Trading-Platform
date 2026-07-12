from __future__ import annotations

import asyncio
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.candle.ws_router import kraken_ohlc_client
from app.candle.ws_router import router as candle_router

logging.basicConfig(level=logging.INFO)


@asynccontextmanager
async def lifespan(app: FastAPI):
    task = asyncio.create_task(kraken_ohlc_client.run())
    try:
        yield
    finally:
        await kraken_ohlc_client.stop()
        task.cancel()


app = FastAPI(title="Paper Trader", lifespan=lifespan)

# Phase 1 dev convenience: Vite's dev server runs on a different origin.
app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:5173"],
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(candle_router)


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}
