package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/lib/pq"
)

var processedOrders atomic.Uint64

func processOrder(ctx context.Context, db *sql.DB) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var id int64
	var item string

	err := db.QueryRowContext(ctx, `
		UPDATE orders
		SET processed = TRUE
		WHERE id = (
			SELECT id
			FROM orders
			WHERE processed = FALSE
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, item
	`).Scan(&id, &item)

	if errors.Is(err, sql.ErrNoRows) {
		return
	}

	if err != nil {
		log.Printf("process order: %v", err)
		return
	}

	processedOrders.Add(1)
	log.Printf("order processed: id=%d item=%q", id, item)
}

func metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintln(w, "# HELP shop_service_info Static information about the shop service.")
	fmt.Fprintln(w, "# TYPE shop_service_info gauge")
	fmt.Fprintln(w, `shop_service_info{service="worker"} 1`)
	fmt.Fprintln(w, "# HELP shop_orders_processed_total Number of orders processed by this worker process.")
	fmt.Fprintln(w, "# TYPE shop_orders_processed_total counter")
	fmt.Fprintf(w, "shop_orders_processed_total{service=\"worker\"} %d\n", processedOrders.Load())
}

func run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()

	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = db.PingContext(checkCtx)
	cancel()

	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", metrics)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ok\n"))
	})

	server := &http.Server{
		Addr:              ":8082",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	log.Println("worker started; health endpoint on :8082")

	for {
		select {
		case <-ticker.C:
			processOrder(ctx, db)

		case err := <-serverErrors:
			return err

		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			defer cancel()

			return server.Shutdown(shutdownCtx)
		}
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
