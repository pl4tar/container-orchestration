package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Open подключается к Postgres по DATABASE_URL или стандартным PG*-переменным.
// Пытается несколько раз при старте — БД может подниматься после пода.
func Open(ctx context.Context) (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=postgres user=postgres password=postgres dbname=shop sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for i := 0; i < 30; i++ {
		if err := db.PingContext(ctx); err == nil {
			return db, nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("db ping failed: %w", lastErr)
}

// InitSchema создаёт таблицу orders, если её нет.
func InitSchema(ctx context.Context, db *sql.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS orders (
    id           SERIAL PRIMARY KEY,
    payload      TEXT NOT NULL,
    processed    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);`
	_, err := db.ExecContext(ctx, ddl)
	return err
}

// Order — одна строка таблицы orders.
type Order struct {
	ID          int        `json:"id"`
	Payload     string     `json:"payload"`
	Processed   bool       `json:"processed"`
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}
