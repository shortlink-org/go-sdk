package tracing

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/shortlink-org/go-sdk/config"
)

// The tests touch the otel globals, so they must not run in parallel with each other.
// Each one restores what it found.
func restoreGlobals(t *testing.T) {
	t.Helper()

	prevProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()

	t.Cleanup(func() {
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
	})
}

func newConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, err := config.New()
	if err != nil {
		t.Fatalf("config.New() returned error: %v", err)
	}

	return cfg
}

// unreachableAddr returns a loopback address nothing listens on.
func unreachableAddr(t *testing.T) string {
	t.Helper()

	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() returned error: %v", err)
	}

	addr := lis.Addr().String()

	err = lis.Close()
	if err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	return addr
}

func TestNewEmptyURIDisablesTracing(t *testing.T) {
	restoreGlobals(t)

	cfg := newConfig(t)
	cfg.Set("TRACER_URI", "")

	before := otel.GetTracerProvider()
	beforePropagator := otel.GetTextMapPropagator()

	tp, cleanup, err := New(context.Background(), slog.New(slog.DiscardHandler), cfg)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if tp != nil {
		t.Fatalf("New() = %T, want nil provider", tp)
	}

	if cleanup == nil {
		t.Fatal("New() returned nil cleanup")
	}

	if got := otel.GetTracerProvider(); got != before {
		t.Fatalf("global provider replaced: %T, want %T", got, before)
	}

	if got := otel.GetTextMapPropagator(); got != beforePropagator {
		t.Fatalf("global propagator replaced: %T, want %T", got, beforePropagator)
	}

	cleanup()
	cleanup()
}

func TestNewInstallsProviderAndShutdownIsBounded(t *testing.T) {
	restoreGlobals(t)

	const shutdownTimeout = 500 * time.Millisecond

	cfg := newConfig(t)
	cfg.Set("TRACER_URI", unreachableAddr(t))
	cfg.Set("TRACING_SHUTDOWN_TIMEOUT", shutdownTimeout.String())

	before := otel.GetTracerProvider()

	// The startup ctx is canceled before cleanup runs, as it is in a real service.
	ctx, cancel := context.WithCancel(context.Background())

	tp, cleanup, err := New(ctx, slog.New(slog.DiscardHandler), cfg)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if tp == nil {
		t.Fatal("New() returned nil provider")
	}

	if got := otel.GetTracerProvider(); got == before {
		t.Fatalf("global provider not replaced: still %T", got)
	}

	// Leave a span pending so cleanup has something to flush to the missing collector.
	_, span := tp.Tracer("test").Start(ctx, "pending")
	span.End()

	cancel()

	start := time.Now()

	cleanup()

	elapsed := time.Since(start)

	// The slack absorbs scheduler noise; the point is that a one-minute
	// TRACING_MAX_ELAPSED_TIME does not leak into shutdown.
	if limit := shutdownTimeout + 2*time.Second; elapsed > limit {
		t.Fatalf("cleanup took %s, want at most %s", elapsed, limit)
	}

	cleanup()
}
