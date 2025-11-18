package http_client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestClientRecords429Metrics(t *testing.T) {
	const delta = 1e-9

	metrics := NewMetrics("test", "client")
	reg := prometheus.NewRegistry()
	require.NoError(t, metrics.Register(reg))

	baseTransport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := new(http.Response)
		resp.Body = io.NopCloser(strings.NewReader(""))
		resp.Header = make(http.Header)
		resp.Request = req
		resp.StatusCode = http.StatusTooManyRequests

		return resp, nil
	})

	client, err := New(
		WithClientName("test-client"),
		WithMetrics(metrics),
		WithBaseTransport(baseTransport),
	)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.example.test/resource", http.NoBody)
	require.NoError(t, err)

	resp, err := client.Transport.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})

	labels := map[string]string{
		LabelClient: "test-client",
		LabelHost:   "api.example.test",
		LabelMethod: http.MethodGet,
	}
	value := counterValue(t, reg, "test_client_rate_limit_429_total", labels)
	require.InDelta(t, 1, value, delta, "429 counter must increment")
}

func counterValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}

		for _, metric := range mf.GetMetric() {
			if matchLabels(metric.GetLabel(), labels) {
				return metric.GetCounter().GetValue()
			}
		}
	}

	t.Fatalf("metric %s with labels %v not found", name, labels)

	return 0
}

func matchLabels(metricLabels []*io_prometheus_client.LabelPair, expected map[string]string) bool {
	if len(metricLabels) != len(expected) {
		return false
	}

	for _, labelPair := range metricLabels {
		value, ok := expected[labelPair.GetName()]
		if !ok || value != labelPair.GetValue() {
			return false
		}
	}

	return true
}
