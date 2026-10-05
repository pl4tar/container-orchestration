package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "path", "status"})

	errorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_request_errors_total",
		Help: "Total number of HTTP requests finished with 5xx.",
	}, []string{"method", "path"})

	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 1.5, 2, 2.5, 3, 5},
	}, []string{"method", "path"})
)

var tracer = otel.Tracer("api")

func main() {
	slog.SetDefault(slog.New(traceHandler{slog.NewJSONHandler(os.Stdout, nil)}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := initTracing(ctx)
	if err != nil {
		slog.Error("init tracing", "error", err)
		os.Exit(1)
	}

	port := getenv("PORT", "8080")

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	handle(mux, "/health", Health())
	handle(mux, "/fail", Fail())
	handle(mux, "/slow", Slow())
	handle(mux, "/load", Load("http://127.0.0.1:"+port+"/health"))

	srv := &http.Server{Addr: ":" + port, Handler: mux}

	go func() {
		slog.Info("starting server", "addr", srv.Addr)
		if e := srv.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
			slog.Error("server stopped", "error", e)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e := srv.Shutdown(shutdownCtx); e != nil {
		slog.Error("server shutdown", "error", e)
	}
	if e := shutdownTracing(shutdownCtx); e != nil {
		slog.Error("tracing shutdown", "error", e)
	}
}

func Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}
}

func Fail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := errors.New("something went wrong")

		span := trace.SpanFromContext(r.Context())
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		slog.ErrorContext(r.Context(), "request failed", "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func Slow() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d := time.Second + rand.N(2*time.Second)

		ctx, span := tracer.Start(r.Context(), "slow-op")
		span.SetAttributes(attribute.Int64("sleep_ms", d.Milliseconds()))
		select {
		case <-time.After(d):
		case <-ctx.Done():
		}
		span.End()

		fmt.Fprintf(w, "slept %s\n", d)
	}
}

// Load fires ?n= requests (default 300) at target with ?c= workers (default 20).
func Load(target string) http.HandlerFunc {
	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   5 * time.Second,
	}

	return func(w http.ResponseWriter, r *http.Request) {
		n := queryInt(r, "n", 300, 10000)
		c := queryInt(r, "c", 20, 200)

		var failed atomic.Int64
		jobs := make(chan struct{})
		var wg sync.WaitGroup
		for range c {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range jobs {
					req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
					resp, err := client.Do(req)
					if err != nil {
						failed.Add(1)
						continue
					}
					resp.Body.Close()
				}
			}()
		}
		for range n {
			jobs <- struct{}{}
		}
		close(jobs)
		wg.Wait()

		slog.InfoContext(r.Context(), "load finished", "requests", n, "failed", failed.Load())
		fmt.Fprintf(w, "sent %d requests, %d failed\n", n, failed.Load())
	}
}

// handle registers h under route with tracing, metrics and access log.
// The route (not the raw URL) is used as the label to keep cardinality bounded.
func handle(mux *http.ServeMux, route string, h http.Handler) {
	instrumented := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		h.ServeHTTP(rec, r)

		elapsed := time.Since(start)
		requestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()
		requestDuration.WithLabelValues(r.Method, route).Observe(elapsed.Seconds())
		if rec.status >= 500 {
			errorsTotal.WithLabelValues(r.Method, route).Inc()
		}

		level := slog.LevelInfo
		if rec.status >= 500 {
			level = slog.LevelError
		}
		slog.Log(r.Context(), level, "request",
			"method", r.Method,
			"path", route,
			"status", rec.status,
			"duration_ms", elapsed.Milliseconds(),
		)
	})

	mux.Handle(route, otelhttp.NewHandler(instrumented, route,
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + route
		}),
	))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// traceHandler adds trace_id/span_id of the current span to every log record.
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, rec slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		rec.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, rec)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}

// initTracing always installs a tracer provider (so logs get a trace_id),
// but exports spans only when an OTLP endpoint is configured via
// OTEL_EXPORTER_OTLP_ENDPOINT / OTEL_EXPORTER_OTLP_TRACES_ENDPOINT.
func initTracing(ctx context.Context) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "api")),
		resource.WithFromEnv(), // OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, err
	}

	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" {
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	} else {
		slog.Warn("OTEL_EXPORTER_OTLP_ENDPOINT is not set, spans are not exported")
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return tp.Shutdown, nil
}

func queryInt(r *http.Request, key string, def, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || v <= 0 {
		return def
	}
	return min(v, max)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
