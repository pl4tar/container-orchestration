package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lab3/internal/db"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	database, err := db.Open(ctx)
	if err != nil {
		slog.Error("cannot connect to db", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.InitSchema(ctx, database); err != nil {
		slog.Error("cannot init schema", "err", err)
		os.Exit(1)
	}

	slog.Info("worker started")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("worker stopped")
			return
		case <-ticker.C:
			if err := processBatch(ctx, database); err != nil {
				slog.Error("process batch failed", "err", err)
			}
		}
	}
}

// processBatch забирает пачку необработанных заказов и помечает их обработанными.
// FOR UPDATE SKIP LOCKED — чтобы несколько реплик worker не толкались за одними
// и теми же строками: каждая реплика берёт свою пачку.
func processBatch(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM orders
		WHERE processed = FALSE
		ORDER BY id
		FOR UPDATE SKIP LOCKED
		LIMIT 10`)
	if err != nil {
		return err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()

	if len(ids) == 0 {
		return nil
	}

	// Помечаем обработанными.
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE orders SET processed = TRUE, processed_at = NOW() WHERE id = $1`, id,
		); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	slog.Info("processed batch", "count", len(ids), "ids", ids)
	return nil
}
