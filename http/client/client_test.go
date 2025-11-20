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
	families, err := reg.Gather()
	require.NoError(t, err)

	var value float64
	found := false

	for _, mf := range families {
		if mf.GetName() != "test_client_rate_limit_429_total" {
			continue
		}

		for _, metric := range mf.GetMetric() {
			if matchLabels(metric.GetLabel(), labels) {
				value = metric.GetCounter().GetValue()
				found = true

				break
			}
		}

		if found {
			break
		}
	}

	require.True(t, found, "metric test_client_rate_limit_429_total with labels %v not found", labels)
	require.InDelta(t, 1, value, delta, "429 counter must increment")
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
