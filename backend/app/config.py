import os

KRAKEN_OHLC_WS_URL = "wss://ws.kraken.com/v2"
KRAKEN_REST_OHLC_URL = "https://api.kraken.com/0/public/OHLC"
DEFAULT_OHLC_INTERVAL_MINUTES = 1

HOST = os.environ.get("HOST", "0.0.0.0")
PORT = int(os.environ.get("PORT", "8000"))
