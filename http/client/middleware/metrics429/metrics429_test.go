package metrics429

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shortlink-org/go-sdk/http/client/internal/types"
	"github.com/stretchr/testify/require"
)

func TestMetrics429Middleware_NoMetrics(t *testing.T) {
	next := types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := new(http.Response)
		resp.Body = io.NopCloser(strings.NewReader(""))
		resp.StatusCode = http.StatusTooManyRequests

		return resp, nil
	})

	mw := Middleware(Config{
		Metrics: nil,
	})

	transport := mw(next)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", http.NoBody)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})
}

func TestMetrics429Middleware_Not429(t *testing.T) {
	metrics := types.NewMetrics("test", "metrics429")
	reg := prometheus.NewRegistry()
	require.NoError(t, metrics.Register(reg))

	next := types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := new(http.Response)
		resp.Body = io.NopCloser(strings.NewReader(""))
		resp.StatusCode = http.StatusOK

		return resp, nil
	})

	mw := Middleware(Config{
		Metrics: metrics,
		Client:  "test-client",
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

	// Verify no 429 metrics were recorded
	families, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range families {
		if mf.GetName() == "test_metrics429_rate_limit_429_total" {
			for _, metric := range mf.GetMetric() {
				require.Equal(t, float64(0), metric.GetCounter().GetValue())
			}
		}
	}
}

func TestMetrics429Middleware_Records429(t *testing.T) {
	metrics := types.NewMetrics("test", "metrics429")
	reg := prometheus.NewRegistry()
	require.NoError(t, metrics.Register(reg))

	next := types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := new(http.Response)
		resp.Body = io.NopCloser(strings.NewReader(""))
		resp.StatusCode = http.StatusTooManyRequests

		return resp, nil
	})

	mw := Middleware(Config{
		Metrics: metrics,
		Client:  "test-client",
	})

	transport := mw(next)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.example.com/resource", http.NoBody)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})

	// Verify 429 metric was recorded
	families, err := reg.Gather()
	require.NoError(t, err)

	var found bool
	for _, mf := range families {
		if mf.GetName() == "test_metrics429_rate_limit_429_total" {
			found = true
			require.Greater(t, len(mf.GetMetric()), 0)
			for _, metric := range mf.GetMetric() {
				require.Equal(t, float64(1), metric.GetCounter().GetValue())
			}
		}
	}

	require.True(t, found, "429 metric should be recorded")
}

func TestMetrics429Middleware_ErrorFromNext(t *testing.T) {
	metrics := types.NewMetrics("test", "metrics429")
	reg := prometheus.NewRegistry()
	require.NoError(t, metrics.Register(reg))

	next := types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, http.ErrServerClosed
	})

	mw := Middleware(Config{
		Metrics: metrics,
		Client:  "test-client",
	})

	transport := mw(next)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", http.NoBody)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, http.ErrServerClosed, err)
}
