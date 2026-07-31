package persistence

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS orders (
    order_id          INTEGER PRIMARY KEY AUTOINCREMENT,
    engine_order_id   INTEGER,
    symbol            TEXT    NOT NULL,
    side              TEXT    NOT NULL,
    order_type        TEXT    NOT NULL,
    price_ticks       INTEGER,
    size_ticks        INTEGER NOT NULL,
    status            TEXT    NOT NULL,
    cancel_reason     TEXT,
    reject_reason     TEXT,
    client_request_id TEXT    NOT NULL,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    expected_fill_ticks INTEGER
);

CREATE TABLE IF NOT EXISTS fills (
    fill_id     INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id    INTEGER NOT NULL REFERENCES orders(order_id),
    price_ticks INTEGER NOT NULL,
    size_ticks  INTEGER NOT NULL,
    ts          INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS engine_state (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    last_instance_id INTEGER NOT NULL
);
`

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("persistence: open %s: %w", path, err)
	}

	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("persistence: %s: %w", pragma, err)
		}
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("persistence: create schema: %w", err)
	}

	return db, nil
}
