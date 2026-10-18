package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
)

type Order struct {
	ID        int64     `json:"id"`
	Item      string    `json:"item"`
	Processed bool      `json:"processed"`
	CreatedAt time.Time `json:"created_at"`
}

type App struct {
	db *sql.DB
}

var ordersCreated atomic.Uint64

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func (app *App) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if strings.EqualFold(os.Getenv("HEALTH_FAIL"), "true") {
		http.Error(w, "health check failed", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("ok\n"))
}

func metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	health := 1
	if strings.EqualFold(os.Getenv("HEALTH_FAIL"), "true") {
		health = 0
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintln(w, "# HELP shop_service_info Static information about the shop service.")
	fmt.Fprintln(w, "# TYPE shop_service_info gauge")
	fmt.Fprintln(w, `shop_service_info{service="api"} 1`)
	fmt.Fprintln(w, "# HELP shop_health_status Whether the service reports itself healthy.")
	fmt.Fprintln(w, "# TYPE shop_health_status gauge")
	fmt.Fprintf(w, "shop_health_status{service=\"api\"} %d\n", health)
	fmt.Fprintln(w, "# HELP shop_orders_created_total Number of orders created by this API process.")
	fmt.Fprintln(w, "# TYPE shop_orders_created_total counter")
	fmt.Fprintf(w, "shop_orders_created_total{service=\"api\"} %d\n", ordersCreated.Load())
}

func (app *App) createOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var input struct {
		Item string `json:"item"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	input.Item = strings.TrimSpace(input.Item)
	if input.Item == "" || len(input.Item) > 200 {
		http.Error(w, "item must contain 1–200 bytes", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var order Order

	err := app.db.QueryRowContext(ctx, `
		INSERT INTO orders (item)
		VALUES ($1)
		RETURNING id, item, processed, created_at
	`, input.Item).Scan(
		&order.ID,
		&order.Item,
		&order.Processed,
		&order.CreatedAt,
	)

	if err != nil {
		log.Printf("create order: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	ordersCreated.Add(1)
	log.Printf("order created: id=%d", order.ID)
	writeJSON(w, http.StatusCreated, order)
}

func (app *App) listOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := app.db.QueryContext(ctx, `
		SELECT id, item, processed, created_at
		FROM orders
		ORDER BY id
	`)
	if err != nil {
		log.Printf("list orders: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	orders := make([]Order, 0)

	for rows.Next() {
		var order Order

		if err := rows.Scan(
			&order.ID,
			&order.Item,
			&order.Processed,
			&order.CreatedAt,
		); err != nil {
			log.Printf("read order: %v", err)
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		log.Printf("iterate orders: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, orders)
}

func run() error {
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return err
	}

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS orders (
			id BIGSERIAL PRIMARY KEY,
			item TEXT NOT NULL,
			processed BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return err
	}

	app := &App{db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", app.health)
	mux.HandleFunc("/metrics", metrics)
	mux.HandleFunc("/order", app.createOrder)
	mux.HandleFunc("/orders", app.listOrders)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Println("api v2 listening on :8080")
	return server.ListenAndServe()
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
