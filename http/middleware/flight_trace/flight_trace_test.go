package flight_trace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shortlink-org/go-sdk/flight_trace"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

const flightDumpDirPerm = 0o700

func setup(t *testing.T) *flight_trace.Recorder {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Clean up /tmp/flight_dumps before each test
	dumpPath := "/tmp/flight_dumps"
	require.NoError(t, os.RemoveAll(dumpPath))
	require.NoError(t, os.MkdirAll(dumpPath, flightDumpDirPerm))

	viper.Set("FLIGHT_RECORDER_DUMP_PATH", dumpPath)
	viper.Set("FLIGHT_TRACE_LATENCY_THRESHOLD", "200ms")

	rec, err := flight_trace.New(ctx)
	require.NoError(t, err)
	require.NotNil(t, rec)

	return rec
}

func countDumps(t *testing.T, path string) int {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(path, "*.out"))
	require.NoError(t, err)

	return len(files)
}

func TestDebugTraceMiddleware_HeaderTrigger(t *testing.T) {
	rec := setup(t)

	var cfg logger.Configuration

	logInstance, err := logger.New(cfg)
	require.NoError(t, err)

	handler := DebugTraceMiddleware(rec, logInstance)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("X-Debug-Trace", "true")

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Eventually(t, func() bool {
		return countDumps(t, "/tmp/flight_dumps") > 0
	}, time.Second, 50*time.Millisecond, "expected dump file to be created")

	files, err := filepath.Glob("/tmp/flight_dumps/trace-*.out")
	require.NoError(t, err)
	require.NotEmpty(t, files, "dump file name should start with trace- and end with .out")
}

func TestDebugTraceMiddleware_SlowRequest(t *testing.T) {
	rec := setup(t)

	var cfg logger.Configuration

	logInstance, err := logger.New(cfg)
	require.NoError(t, err)

	handler := DebugTraceMiddleware(rec, logInstance)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond) // exceed threshold
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Eventually(t, func() bool {
		return countDumps(t, "/tmp/flight_dumps") > 0
	}, time.Second, 50*time.Millisecond, "expected dump file to be created for slow request")
}

func TestDebugTraceMiddleware_NoTrigger(t *testing.T) {
	rec := setup(t)

	var cfg logger.Configuration

	logInstance, err := logger.New(cfg)
	require.NoError(t, err)

	handler := DebugTraceMiddleware(rec, logInstance)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond) // faster than threshold
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	time.Sleep(300 * time.Millisecond)
	require.Equal(t, 0, countDumps(t, "/tmp/flight_dumps"), "no dump files expected")
}
