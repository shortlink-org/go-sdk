package deadline

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shortlink-org/go-sdk/http/client/internal/types"
	"github.com/stretchr/testify/require"
)

func TestDeadlineMiddleware_NoDeadline(t *testing.T) {
	next := types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := new(http.Response)
		resp.Body = io.NopCloser(strings.NewReader("ok"))
		resp.StatusCode = http.StatusOK

		return resp, nil
	})

	mw := Middleware(Config{
		Threshold: 100 * time.Millisecond,
	})

	transport := mw(next)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", http.NoBody)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})
}

func TestDeadlineMiddleware_DeadlineTooClose(t *testing.T) {
	next := types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("next should not be called")

		return nil, nil
	})

	mw := Middleware(Config{
		Threshold: 100 * time.Millisecond,
	})

	transport := mw(next)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(50*time.Millisecond))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, types.ErrDeadlineTooClose, err)
}

func TestDeadlineMiddleware_DeadlineFarEnough(t *testing.T) {
	next := types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := new(http.Response)
		resp.Body = io.NopCloser(strings.NewReader("ok"))
		resp.StatusCode = http.StatusOK

		return resp, nil
	})

	mw := Middleware(Config{
		Threshold: 100 * time.Millisecond,
	})

	transport := mw(next)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(500*time.Millisecond))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})
}

func TestDeadlineMiddleware_ZeroThreshold(t *testing.T) {
	next := types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := new(http.Response)
		resp.Body = io.NopCloser(strings.NewReader("ok"))
		resp.StatusCode = http.StatusOK

		return resp, nil
	})

	mw := Middleware(Config{
		Threshold: 0,
	})

	transport := mw(next)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(50*time.Millisecond))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})
}

func TestDeadlineMiddleware_Metrics(t *testing.T) {
	metrics := types.NewMetrics("test", "deadline")
	reg := prometheus.NewRegistry()
	require.NoError(t, metrics.Register(reg))

	next := types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("next should not be called")

		return nil, nil
	})

	mw := Middleware(Config{
		Threshold: 100 * time.Millisecond,
		Metrics:   metrics,
		Client:    "test-client",
	})

	transport := mw(next)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(50*time.Millisecond))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com/test", http.NoBody)
	require.NoError(t, err)

	_, err = transport.RoundTrip(req)
	require.Error(t, err)

	families, err := reg.Gather()
	require.NoError(t, err)

	var found bool
	for _, mf := range families {
		if mf.GetName() == "test_deadline_deadline_canceled_total" {
			found = true
			require.Greater(t, len(mf.GetMetric()), 0)
		}
	}

	require.True(t, found, "deadline_canceled_total metric should be recorded")
}
