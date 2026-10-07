package main

import (
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	httpErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_errors_total",
			Help: "Total number of HTTP errors.",
		},
		[]string{"method", "path"},
	)

	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(httpRequests)
	prometheus.MustRegister(httpErrors)
	prometheus.MustRegister(httpDuration)
}

func observe(
	method string,
	path string,
	status int,
	duration time.Duration,
) {
	httpRequests.WithLabelValues(
		method,
		path,
		fmt.Sprint(status),
	).Inc()

	if status >= 500 {
		httpErrors.WithLabelValues(
			method,
			path,
		).Inc()
	}

	httpDuration.WithLabelValues(
		method,
		path,
	).Observe(duration.Seconds())

	fields := []any{
		"method", method,
		"path", path,
		"status", status,
		"duration_ms", float64(duration) / float64(time.Millisecond),
	}

	if status >= 500 {
		slog.Error("request completed", fields...)
	} else {
		slog.Info("request completed", fields...)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))

	observe(
		r.Method,
		"/health",
		http.StatusOK,
		time.Since(start),
	)
}

func failHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte("internal server error\n"))

	observe(
		r.Method,
		"/fail",
		http.StatusInternalServerError,
		time.Since(start),
	)
}

func slowHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	delay := time.Duration(1+rand.Intn(3)) * time.Second
	time.Sleep(delay)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("slow response\n"))

	observe(
		r.Method,
		"/slow",
		http.StatusOK,
		time.Since(start),
	)
}

func loadHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for i := 0; i < 20; i++ {
		resp, err := client.Get("http://127.0.0.1:8080/health")

		if err != nil {
			slog.Error(
				"load request failed",
				"path", "/load",
				"error", err.Error(),
			)
			continue
		}

		resp.Body.Close()
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("load generated\n"))

	observe(
		r.Method,
		"/load",
		http.StatusOK,
		time.Since(start),
	)
}

func main() {
	logger := slog.New(
		slog.NewJSONHandler(os.Stdout, nil),
	)
	slog.SetDefault(logger)

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/fail", failHandler)
	http.HandleFunc("/slow", slowHandler)
	http.HandleFunc("/load", loadHandler)

	http.Handle("/metrics", promhttp.Handler())

	slog.Info("api is listening", "address", ":8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		slog.Error("server stopped", "error", err.Error())
		os.Exit(1)
	}
}
