from pydantic import BaseModel


class CandleMessage(BaseModel):
    type: str = "candle"
    symbol: str
    interval: str
    time: int
    open: float
    high: float
    low: float
    close: float
    volume: float
    closed: bool
