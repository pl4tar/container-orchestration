package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"

	"lab3/internal/db"
)

func main() {
	ctx := context.Background()

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

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/order", createOrderHandler(database))
	mux.HandleFunc("/orders", listOrdersHandler(database))

	slog.Info("api listening", "addr", ":8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("HEALTH_FAIL") == "true" {
		http.Error(w, "health check failed", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func createOrderHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body failed", http.StatusBadRequest)
			return
		}
		payload := string(body)
		var parsed struct {
			Payload string `json:"payload"`
		}
		if json.Unmarshal(body, &parsed) == nil && parsed.Payload != "" {
			payload = parsed.Payload
		}
		if payload == "" {
			http.Error(w, "empty payload", http.StatusBadRequest)
			return
		}
		var id int
		err = database.QueryRowContext(r.Context(),
			`INSERT INTO orders (payload) VALUES ($1) RETURNING id`, payload,
		).Scan(&id)
		if err != nil {
			http.Error(w, "insert failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "payload": payload})
	}
}

func listOrdersHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rows, err := database.QueryContext(r.Context(),
			`SELECT id, payload, processed, created_at, processed_at
			 FROM orders ORDER BY id DESC LIMIT 200`)
		if err != nil {
			http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		out := make([]db.Order, 0)
		for rows.Next() {
			var o db.Order
			if err := rows.Scan(&o.ID, &o.Payload, &o.Processed, &o.CreatedAt, &o.ProcessedAt); err != nil {
				http.Error(w, "scan failed", http.StatusInternalServerError)
				return
			}
			out = append(out, o)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}
