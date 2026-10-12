package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
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
	ctx context.Context,
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
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		fields = append(fields, "trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
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
		r.Context(),
		r.Method,
		"/health",
		http.StatusOK,
		time.Since(start),
	)
}

func failHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	span := trace.SpanFromContext(r.Context())
	span.RecordError(errors.New("intentional failure from /fail"))
	span.SetStatus(codes.Error, "intentional HTTP 500")

	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte("internal server error\n"))

	observe(
		r.Context(),
		r.Method,
		"/fail",
		http.StatusInternalServerError,
		time.Since(start),
	)
}

func slowHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	delay := time.Duration(1+rand.Intn(3)) * time.Second
	_, span := otel.Tracer("lab2/api").Start(r.Context(), "slow-op")
	span.SetAttributes(attribute.Int64("sleep.duration_ms", delay.Milliseconds()))
	time.Sleep(delay)
	span.End()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("slow response\n"))

	observe(
		r.Context(),
		r.Method,
		"/slow",
		http.StatusOK,
		time.Since(start),
	)
}

func loadHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	for i := 0; i < 20; i++ {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://127.0.0.1:8080/health", nil)
		if err != nil {
			trace.SpanFromContext(r.Context()).RecordError(err)
			continue
		}
		resp, err := client.Do(req)

		if err != nil {
			slog.Error(
				"load request failed",
				"path", "/load",
				"error", err.Error(),
				"trace_id", trace.SpanContextFromContext(r.Context()).TraceID().String(),
			)
			continue
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("load generated\n"))

	observe(
		r.Context(),
		r.Method,
		"/load",
		http.StatusOK,
		time.Since(start),
	)
}

func setupTracing(ctx context.Context) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "api"
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		// For this lab retain every request, so each log can be matched to a trace.
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", serviceName))),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return provider, nil
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	for path, handler := range map[string]http.HandlerFunc{
		"/health": healthHandler,
		"/fail":   failHandler,
		"/slow":   slowHandler,
		"/load":   loadHandler,
	} {
		mux.Handle(path, otelhttp.NewHandler(handler, "GET "+path))
	}
	// Prometheus scrapes should not flood the trace store.
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	provider, err := setupTracing(ctx)
	if err != nil {
		return fmt.Errorf("initialize tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Shutdown(shutdownCtx); err != nil {
			slog.Error("flush traces failed", "error", err.Error())
		}
	}()

	server := &http.Server{Addr: ":8080", Handler: newHandler(), ReadHeaderTimeout: 5 * time.Second}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	slog.Info("api is listening", "address", ":8080")
	select {
	case err := <-serverErrors:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func main() {
	logger := slog.New(
		slog.NewJSONHandler(os.Stdout, nil),
	)
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("server stopped", "error", err.Error())
		os.Exit(1)
	}
}
