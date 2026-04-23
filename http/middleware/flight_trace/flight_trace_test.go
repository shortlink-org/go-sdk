package flight_trace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/flight_trace"
	"github.com/shortlink-org/go-sdk/logger"
)

// The runtime allows only one active flight recorder; serialize tests and
// wait for Stop after each cancel so the next New can call Start safely.
var flightRecorderTestMu sync.Mutex

func TestDebugTraceMiddleware_HeaderTrigger(t *testing.T) {
	flightRecorderTestMu.Lock()
	t.Cleanup(func() {
		time.Sleep(50 * time.Millisecond)
		flightRecorderTestMu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	dumpPath := t.TempDir()

	cfg, err := config.New()
	require.NoError(t, err)

	cfg.Set("FLIGHT_RECORDER_DUMP_PATH", dumpPath)
	cfg.Set("FLIGHT_TRACE_LATENCY_THRESHOLD", "200ms")

	rec, err := flight_trace.New(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, rec)

	var loggerCfg logger.Configuration

	logInstance, err := logger.New(loggerCfg)
	require.NoError(t, err)

	handler := DebugTraceMiddleware(rec, logInstance, cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", http.NoBody)
	req.Header.Set("X-Debug-Trace", "true")

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Eventually(t, func() bool {
		files, globErr := filepath.Glob(filepath.Join(dumpPath, "*.out"))
		require.NoError(t, globErr)

		return len(files) > 0
	}, time.Second, 50*time.Millisecond, "expected dump file to be created")

	files, err := filepath.Glob(filepath.Join(dumpPath, "trace-*.out"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "dump file name should start with trace- and end with .out")
}

func TestDebugTraceMiddleware_SlowRequest(t *testing.T) {
	flightRecorderTestMu.Lock()
	t.Cleanup(func() {
		time.Sleep(50 * time.Millisecond)
		flightRecorderTestMu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	dumpPath := t.TempDir()

	cfg, err := config.New()
	require.NoError(t, err)

	cfg.Set("FLIGHT_RECORDER_DUMP_PATH", dumpPath)
	cfg.Set("FLIGHT_TRACE_LATENCY_THRESHOLD", "200ms")

	rec, err := flight_trace.New(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, rec)

	var loggerCfg logger.Configuration

	logInstance, err := logger.New(loggerCfg)
	require.NoError(t, err)

	handler := DebugTraceMiddleware(rec, logInstance, cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond) // exceed threshold
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Eventually(t, func() bool {
		files, globErr := filepath.Glob(filepath.Join(dumpPath, "*.out"))
		require.NoError(t, globErr)

		return len(files) > 0
	}, time.Second, 50*time.Millisecond, "expected dump file to be created for slow request")
}

func TestDebugTraceMiddleware_NoTrigger(t *testing.T) {
	flightRecorderTestMu.Lock()
	t.Cleanup(func() {
		time.Sleep(50 * time.Millisecond)
		flightRecorderTestMu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	dumpPath := t.TempDir()

	cfg, err := config.New()
	require.NoError(t, err)

	cfg.Set("FLIGHT_RECORDER_DUMP_PATH", dumpPath)
	cfg.Set("FLIGHT_TRACE_LATENCY_THRESHOLD", "200ms")

	rec, err := flight_trace.New(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, rec)

	var loggerCfg logger.Configuration

	logInstance, err := logger.New(loggerCfg)
	require.NoError(t, err)

	handler := DebugTraceMiddleware(rec, logInstance, cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond) // faster than threshold
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	time.Sleep(300 * time.Millisecond)

	files, err := filepath.Glob(filepath.Join(dumpPath, "*.out"))
	require.NoError(t, err)
	require.Empty(t, files, "no dump files expected")
}
