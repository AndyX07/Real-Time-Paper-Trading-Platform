package persistence

import (
	"database/sql"
	"fmt"
	"math/big"
	"time"
)

type Order struct {
	OrderID         int64
	EngineOrderID   sql.NullInt64
	Symbol          string
	Side            string // "buy" | "sell"
	OrderType       string // "market" | "limit"
	PriceTicks      sql.NullInt64
	SizeTicks       int64
	Status          string
	CancelReason    sql.NullString
	RejectReason    sql.NullString
	ClientRequestID string
	CreatedAt       int64
	UpdatedAt       int64
}

type Position struct {
	Symbol           string
	NetSizeTicks     int64 // positive = net long, negative = net short
	AvgCostTicks     int64 // weighted average price of the currently open lots only, in price ticks
	RealizedPnLTicks int64 // cumulative P&L from closed lots, at the same 1e10 scale as price/size ticks
}

const tickScale = 10_000_000_000 // 1e10

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreatePendingOrder(symbol, side, orderType string, priceTicks *int64, sizeTicks int64,
	clientRequestID string) (int64, error) {
	now := time.Now().UnixNano()
	var price sql.NullInt64
	if priceTicks != nil {
		price = sql.NullInt64{Int64: *priceTicks, Valid: true}
	}

	res, err := r.db.Exec(
		`INSERT INTO orders (engine_order_id, symbol, side, order_type, price_ticks, size_ticks, status, client_request_id, created_at, updated_at)
		 VALUES (NULL, ?, ?, ?, ?, ?, 'pending', ?, ?, ?)`,
		symbol, side, orderType, price, sizeTicks, clientRequestID, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("persistence: create pending order: %w", err)
	}
	return res.LastInsertId()
}

func (r *Repository) MarkOrderAccepted(orderID int64, engineOrderID uint64, expectedFillTicks int64) error {
	status := "open"
	engineOrderIDToStore := sql.NullInt64{Int64: int64(engineOrderID), Valid: true}
	if expectedFillTicks <= 0 {
		status = "unfilled"
		engineOrderIDToStore = sql.NullInt64{} // terminal -- same reasoning as MarkOrderCanceled
	}
	_, err := r.db.Exec(
		`UPDATE orders SET status = ?, engine_order_id = ?, expected_fill_ticks = ?, updated_at = ? WHERE order_id = ?`,
		status, engineOrderIDToStore, expectedFillTicks, time.Now().UnixNano(), orderID,
	)
	if err != nil {
		return fmt.Errorf("persistence: mark order accepted: %w", err)
	}
	return nil
}

func (r *Repository) MarkOrderRejected(orderID int64, reason string) error {
	_, err := r.db.Exec(
		`UPDATE orders SET status = 'rejected', reject_reason = ?, updated_at = ? WHERE order_id = ?`,
		reason, time.Now().UnixNano(), orderID,
	)
	if err != nil {
		return fmt.Errorf("persistence: mark order rejected: %w", err)
	}
	return nil
}

func (r *Repository) MarkOrderCanceled(orderID int64, reason string) error {
	_, err := r.db.Exec(
		`UPDATE orders SET status = 'canceled', cancel_reason = ?, engine_order_id = NULL, updated_at = ? WHERE order_id = ?`,
		reason, time.Now().UnixNano(), orderID,
	)
	if err != nil {
		return fmt.Errorf("persistence: mark order canceled: %w", err)
	}
	return nil
}

func (r *Repository) GetOrder(orderID int64) (Order, error) {
	var o Order
	err := r.db.QueryRow(
		`SELECT order_id, engine_order_id, symbol, side, order_type, price_ticks, size_ticks, status,
		        cancel_reason, reject_reason, client_request_id, created_at, updated_at
		 FROM orders WHERE order_id = ?`, orderID,
	).Scan(&o.OrderID, &o.EngineOrderID, &o.Symbol, &o.Side, &o.OrderType, &o.PriceTicks, &o.SizeTicks, &o.Status,
		&o.CancelReason, &o.RejectReason, &o.ClientRequestID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return Order{}, fmt.Errorf("persistence: get order %d: %w", orderID, err)
	}
	return o, nil
}

// ErrOrderNotFound is returned by RecordFill when engineOrderID doesn't
// resolve to a local order yet -- can genuinely happen for a market
// order's immediate fill, which can reach the fill watcher before the
// PlaceOrder RPC's own accept response finishes being persisted
// (WatchFills is a separate, already-open stream with no ordering
// relationship to a concurrent unary call's reply). Callers should retry
// briefly rather than treat this as permanent, the same way the book
// path tolerates a delta arriving before its own seed snapshot.
var ErrOrderNotFound = fmt.Errorf("persistence: no order for that engine_order_id")

func (r *Repository) RecordFill(engineOrderID uint64, priceTicks, sizeTicks int64, ts int64) (orderID int64, err error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("persistence: record fill: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var localOrderID, expectedFillTicks int64
	err = tx.QueryRow(`SELECT order_id, expected_fill_ticks FROM orders WHERE engine_order_id = ?`, int64(engineOrderID)).
		Scan(&localOrderID, &expectedFillTicks)
	if err == sql.ErrNoRows {
		err = ErrOrderNotFound
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("persistence: record fill: lookup order: %w", err)
	}

	if _, err = tx.Exec(
		`INSERT INTO fills (order_id, price_ticks, size_ticks, ts) VALUES (?, ?, ?, ?)`,
		localOrderID, priceTicks, sizeTicks, ts,
	); err != nil {
		return 0, fmt.Errorf("persistence: record fill: insert: %w", err)
	}

	var totalFilled int64
	if err = tx.QueryRow(`SELECT COALESCE(SUM(size_ticks), 0) FROM fills WHERE order_id = ?`, localOrderID).
		Scan(&totalFilled); err != nil {
		return 0, fmt.Errorf("persistence: record fill: sum fills: %w", err)
	}

	// Once fully filled, no further fill will ever arrive for this
	// engine_order_id -- clear it (same reasoning as MarkOrderCanceled)
	// rather than leaving a resolved row that a later engine restart's
	// reused id could collide with. Left intact while partially_filled,
	// since more fills against this exact order are still expected.
	status := "partially_filled"
	keepEngineOrderID := sql.NullInt64{Int64: int64(engineOrderID), Valid: true}
	if totalFilled >= expectedFillTicks {
		status = "filled"
		keepEngineOrderID = sql.NullInt64{}
	}
	if _, err = tx.Exec(`UPDATE orders SET status = ?, engine_order_id = ?, updated_at = ? WHERE order_id = ?`,
		status, keepEngineOrderID, time.Now().UnixNano(), localOrderID); err != nil {
		return 0, fmt.Errorf("persistence: record fill: update status: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("persistence: record fill: commit: %w", err)
	}
	return localOrderID, nil
}

func (r *Repository) GetOpenOrders() ([]Order, error) {
	rows, err := r.db.Query(
		`SELECT order_id, engine_order_id, symbol, side, order_type, price_ticks, size_ticks, status,
		        cancel_reason, reject_reason, client_request_id, created_at, updated_at
		 FROM orders WHERE status IN ('pending', 'open', 'partially_filled')`,
	)
	if err != nil {
		return nil, fmt.Errorf("persistence: get open orders: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.OrderID, &o.EngineOrderID, &o.Symbol, &o.Side, &o.OrderType, &o.PriceTicks,
			&o.SizeTicks, &o.Status, &o.CancelReason, &o.RejectReason, &o.ClientRequestID, &o.CreatedAt,
			&o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("persistence: get open orders: scan: %w", err)
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func (r *Repository) GetAllOrders() ([]Order, error) {
	rows, err := r.db.Query(
		`SELECT order_id, engine_order_id, symbol, side, order_type, price_ticks, size_ticks, status,
		        cancel_reason, reject_reason, client_request_id, created_at, updated_at
		 FROM orders ORDER BY order_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("persistence: get all orders: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.OrderID, &o.EngineOrderID, &o.Symbol, &o.Side, &o.OrderType, &o.PriceTicks,
			&o.SizeTicks, &o.Status, &o.CancelReason, &o.RejectReason, &o.ClientRequestID, &o.CreatedAt,
			&o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("persistence: get all orders: scan: %w", err)
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

type Fill struct {
	OrderID    int64
	Symbol     string
	Side       string
	PriceTicks int64
	SizeTicks  int64
	Ts         int64
}

func (r *Repository) GetFills() ([]Fill, error) {
	rows, err := r.db.Query(
		`SELECT f.order_id, o.symbol, o.side, f.price_ticks, f.size_ticks, f.ts
		 FROM fills f JOIN orders o ON f.order_id = o.order_id
		 ORDER BY f.fill_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("persistence: get fills: %w", err)
	}
	defer rows.Close()

	var fills []Fill
	for rows.Next() {
		var f Fill
		if err := rows.Scan(&f.OrderID, &f.Symbol, &f.Side, &f.PriceTicks, &f.SizeTicks, &f.Ts); err != nil {
			return nil, fmt.Errorf("persistence: get fills: scan: %w", err)
		}
		fills = append(fills, f)
	}
	return fills, rows.Err()
}

// lot is one still-open (partially or fully unmatched) fill, oldest first.
type lot struct {
	priceTicks int64
	sizeTicks  int64
}

func (r *Repository) GetPositions() ([]Position, error) {
	rows, err := r.db.Query(
		`SELECT o.symbol, o.side, f.price_ticks, f.size_ticks
		 FROM fills f JOIN orders o ON f.order_id = o.order_id
		 ORDER BY f.fill_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("persistence: get positions: %w", err)
	}
	defer rows.Close()

	type accum struct {
		side        string // side of the open lots below
		lots        []lot  // FIFO queue, oldest at index 0
		realizedPnl *big.Int
	}
	bySymbol := make(map[string]*accum)

	for rows.Next() {
		var symbol, side string
		var priceTicks, sizeTicks int64
		if err := rows.Scan(&symbol, &side, &priceTicks, &sizeTicks); err != nil {
			return nil, fmt.Errorf("persistence: get positions: scan: %w", err)
		}

		a, ok := bySymbol[symbol]
		if !ok {
			a = &accum{realizedPnl: new(big.Int)}
			bySymbol[symbol] = a
		}
		if len(a.lots) == 0 {
			a.side = side
		}

		if side == a.side {
			a.lots = append(a.lots, lot{priceTicks: priceTicks, sizeTicks: sizeTicks})
			continue
		}

		// Opposite direction: closes existing lots oldest-first, realizing
		// P&L on each matched portion against that lot's own price.
		remaining := sizeTicks
		for remaining > 0 && len(a.lots) > 0 {
			open := &a.lots[0]
			matched := min(remaining, open.sizeTicks)

			var diff int64
			if a.side == "sell" {
				diff = open.priceTicks - priceTicks // short closed by a buy
			} else {
				diff = priceTicks - open.priceTicks // long closed by a sell
			}
			pnl := new(big.Int).Mul(big.NewInt(matched), big.NewInt(diff))
			a.realizedPnl.Add(a.realizedPnl, pnl)

			open.sizeTicks -= matched
			remaining -= matched
			if open.sizeTicks == 0 {
				a.lots = a.lots[1:]
			}
		}

		if remaining > 0 {
			// Outsized every open lot -- the position flips sides, and the
			// leftover opens a fresh lot on the new side.
			a.side = side
			a.lots = append(a.lots, lot{priceTicks: priceTicks, sizeTicks: remaining})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("persistence: get positions: %w", err)
	}

	positions := make([]Position, 0, len(bySymbol))
	for symbol, a := range bySymbol {
		if len(a.lots) == 0 && a.realizedPnl.Sign() == 0 {
			continue
		}

		var netSize, avgCost int64
		if len(a.lots) > 0 {
			var totalOpenSize int64
			costSum := new(big.Int)
			for _, l := range a.lots {
				totalOpenSize += l.sizeTicks
				costSum.Add(costSum, new(big.Int).Mul(big.NewInt(l.priceTicks), big.NewInt(l.sizeTicks)))
			}
			avgCost = new(big.Int).Div(costSum, big.NewInt(totalOpenSize)).Int64()
			netSize = totalOpenSize
			if a.side == "sell" {
				netSize = -netSize
			}
		}

		// realizedPnl accumulated as priceTicks*sizeTicks products, an
		// extra factor of tickScale versus a plain tick value -- divide it
		// back down. Quo (truncating), not Div (Euclidean), since this can
		// be negative and truncation-toward-zero matches plain int64 "/".
		realizedPnl := new(big.Int).Quo(a.realizedPnl, big.NewInt(tickScale)).Int64()

		positions = append(positions, Position{
			Symbol: symbol, NetSizeTicks: netSize, AvgCostTicks: avgCost, RealizedPnLTicks: realizedPnl,
		})
	}
	return positions, nil
}

func (r *Repository) GetLastEngineInstanceID() (id uint64, ok bool, err error) {
	var v int64
	err = r.db.QueryRow(`SELECT last_instance_id FROM engine_state WHERE id = 1`).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("persistence: get last engine instance id: %w", err)
	}
	return uint64(v), true, nil
}

func (r *Repository) SetLastEngineInstanceID(id uint64) error {
	_, err := r.db.Exec(
		`INSERT INTO engine_state (id, last_instance_id) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET last_instance_id = excluded.last_instance_id`,
		int64(id),
	)
	if err != nil {
		return fmt.Errorf("persistence: set last engine instance id: %w", err)
	}
	return nil
}

func (r *Repository) CancelOpenOrdersForEngineRestart() ([]Order, error) {
	rows, err := r.db.Query(
		`UPDATE orders SET status = 'canceled', cancel_reason = 'engine_restart', engine_order_id = NULL, updated_at = ?
		 WHERE status IN ('pending', 'open', 'partially_filled')
		 RETURNING order_id, engine_order_id, symbol, side, order_type, price_ticks, size_ticks, status,
		           cancel_reason, reject_reason, client_request_id, created_at, updated_at`,
		time.Now().UnixNano(),
	)
	if err != nil {
		return nil, fmt.Errorf("persistence: cancel open orders for engine restart: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.OrderID, &o.EngineOrderID, &o.Symbol, &o.Side, &o.OrderType, &o.PriceTicks,
			&o.SizeTicks, &o.Status, &o.CancelReason, &o.RejectReason, &o.ClientRequestID, &o.CreatedAt,
			&o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("persistence: cancel open orders for engine restart: scan: %w", err)
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}
