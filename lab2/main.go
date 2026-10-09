package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	errorsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "http_errors_total",
			Help: "Total number of HTTP 5xx errors.",
		},
	)

	responseDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_response_duration_seconds",
			Help:    "HTTP response duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	requestID uint64
)

func init() {
	prometheus.MustRegister(requestsTotal)
	prometheus.MustRegister(errorsTotal)
	prometheus.MustRegister(responseDuration)

	rand.Seed(time.Now().UnixNano())
}

type logEntry struct {
	Time      string `json:"time"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Method    string `json:"method,omitempty"`
	Path      string `json:"path,omitempty"`
	Status    int    `json:"status,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	RequestID uint64 `json:"request_id,omitempty"`
}

func writeLog(ctx context.Context, level, message, method, path string, status int) {
	spanCtx := trace.SpanContextFromContext(ctx)

	entry := logEntry{
		Time:      time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Message:   message,
		Method:    method,
		Path:      path,
		Status:    status,
		RequestID: atomic.AddUint64(&requestID, 1),
	}

	if spanCtx.IsValid() {
		entry.TraceID = spanCtx.TraceID().String()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf(`{"level":"error","message":"failed to encode log"}`)
		return
	}

	fmt.Println(string(data))
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}

	return r.ResponseWriter.Write(body)
}

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		recorder := &responseRecorder{
			ResponseWriter: w,
		}

		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}

		duration := time.Since(start).Seconds()

		requestsTotal.WithLabelValues(
			r.Method,
			r.URL.Path,
			fmt.Sprintf("%d", status),
		).Inc()

		responseDuration.WithLabelValues(
			r.Method,
			r.URL.Path,
		).Observe(duration)

		if status >= 500 {
			errorsTotal.Inc()
		}

		writeLog(
			r.Context(),
			"info",
			"request completed",
			r.Method,
			r.URL.Path,
			status,
		)
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func failHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	span := trace.SpanFromContext(ctx)
	span.SetStatus(codes.Error, "intentional failure")
	span.SetAttributes(attribute.String("error.type", "intentional"))

	writeLog(
		ctx,
		"error",
		"intentional server failure",
		r.Method,
		r.URL.Path,
		http.StatusInternalServerError,
	)

	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func slowHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tracer := otel.Tracer("api")

	_, span := tracer.Start(ctx, "slow-op")
	defer span.End()

	delay := time.Duration(1+rand.Intn(3)) * time.Second

	span.SetAttributes(
		attribute.Int64("sleep.duration_ms", delay.Milliseconds()),
	)

	writeLog(
		ctx,
		"info",
		fmt.Sprintf("starting slow operation for %s", delay),
		r.Method,
		r.URL.Path,
		http.StatusOK,
	)

	select {
	case <-time.After(delay):
		// Operation completed.
	case <-ctx.Done():
		span.SetStatus(codes.Error, "request cancelled")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("slow response\n"))
}

func loadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	const requests = 20

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   5 * time.Second,
	}

	for i := 0; i < requests; i++ {
		go func() {
			req, err := http.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"http://127.0.0.1:8080/health",
				nil,
			)
			if err != nil {
				return
			}

			resp, err := client.Do(req)
			if err != nil {
				return
			}

			_ = resp.Body.Close()
		}()
	}

	writeLog(
		ctx,
		"info",
		fmt.Sprintf("generated %d requests", requests),
		r.Method,
		r.URL.Path,
		http.StatusOK,
	)

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "generated %d requests\n", requests)
}

func setupTracerProvider(ctx context.Context) (*sdktrace.TracerProvider, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	if endpoint == "" {
		log.Println(`{"level":"info","message":"OTEL_EXPORTER_OTLP_ENDPOINT is not set; tracing exporter disabled"}`)

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(
				resource.NewWithAttributes(
					"",
					attribute.String("service.name", "api"),
				),
			),
		)

		otel.SetTracerProvider(tp)

		return tp, nil
	}

	exporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}

	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			attribute.String("service.name", "api"),
			attribute.String("service.version", "1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	return tp, nil
}

func main() {
	ctx := context.Background()

	tracerProvider, err := setupTracerProvider(ctx)
	if err != nil {
		log.Fatalf("failed to initialize tracing: %v", err)
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			log.Printf("failed to shutdown tracer provider: %v", err)
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/fail", failHandler)
	mux.HandleFunc("/slow", slowHandler)
	mux.HandleFunc("/load", loadHandler)

	mux.Handle(
		"/metrics",
		promhttp.Handler(),
	)

	handler := metricsMiddleware(mux)

	handler = otelhttp.NewHandler(
		handler,
		"api",
	)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println(`{"level":"info","message":"api service started","address":":8080"}`)

	if err := server.ListenAndServe(); err != nil &&
		err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}
