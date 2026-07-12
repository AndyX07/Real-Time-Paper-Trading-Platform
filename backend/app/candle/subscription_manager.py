from starlette.websockets import WebSocket

SubKey = tuple[str, int]


class CandleSubscriptionManager:
    def __init__(self) -> None:
        self._clients_by_key: dict[SubKey, set[WebSocket]] = {}

    def subscribe(self, symbol: str, interval_minutes: int, client: WebSocket) -> bool:
        """Returns True if this is the (symbol, interval)'s first subscriber (0 -> 1)."""
        key = (symbol, interval_minutes)
        clients = self._clients_by_key.setdefault(key, set())
        is_new = len(clients) == 0
        clients.add(client)
        return is_new

    def unsubscribe(self, symbol: str, interval_minutes: int, client: WebSocket) -> bool:
        """Returns True if this was the (symbol, interval)'s last subscriber (-> 0)."""
        key = (symbol, interval_minutes)
        clients = self._clients_by_key.get(key)
        if not clients or client not in clients:
            return False
        clients.discard(client)
        if not clients:
            del self._clients_by_key[key]
            return True
        return False

    def unsubscribe_all(self, client: WebSocket) -> list[SubKey]:
        """Called on client disconnect. Returns keys that lost their last subscriber."""
        emptied = []
        for key in list(self._clients_by_key.keys()):
            symbol, interval_minutes = key
            if self.unsubscribe(symbol, interval_minutes, client):
                emptied.append(key)
        return emptied

    def clients_for(self, symbol: str, interval_minutes: int) -> set[WebSocket]:
        return self._clients_by_key.get((symbol, interval_minutes), set())
