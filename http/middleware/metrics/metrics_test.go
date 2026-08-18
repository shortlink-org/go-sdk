package metrics_middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func Test_NewMetrics(t *testing.T) {
	const delta = 1e-9

	registry := prometheus.NewRegistry()

	middlewares, err := NewMetrics(registry)
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Use(middlewares)
	router.Get("/users/{firstName}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/users/bob", http.NoBody)
	router.ServeHTTP(httptest.NewRecorder(), req)

	resp, err := registry.Gather()
	require.NoError(t, err)

	counter := findMetricFamily(t, resp, "http_requests_total")
	require.Len(t, counter.GetMetric(), 1)

	labels := counter.GetMetric()[0].GetLabel()
	require.Equal(t, "200", getValueForLabel(labels, "status"))
	require.Equal(t, http.MethodGet, getValueForLabel(labels, "method"))
	require.Equal(t, "/users/{firstName}", getValueForLabel(labels, "path"))
	require.InDelta(t, 1, counter.GetMetric()[0].GetCounter().GetValue(), delta)

	histogram := findMetricFamily(t, resp, "http_request_duration_seconds")
	require.Len(t, histogram.GetMetric(), 1)
	require.Equal(t, uint64(1), histogram.GetMetric()[0].GetHistogram().GetSampleCount())
	require.Equal(t, "/users/{firstName}", getValueForLabel(histogram.GetMetric()[0].GetLabel(), "path"))
}

// Test_NewMetrics_routePattern pins the label down to chi's own RoutePattern:
// consecutive wildcards collapse and the trailing slash goes away, so mounted
// subrouters do not each contribute their own series.
func Test_NewMetrics_routePattern(t *testing.T) {
	tests := []struct {
		name    string
		mount   string
		route   string
		request string
		want    string
	}{
		{
			name:    "mounted subrouter",
			mount:   "/api",
			route:   "/users/{firstName}",
			request: "/api/users/bob",
			want:    "/api/users/{firstName}",
		},
		{
			name:    "trailing slash",
			mount:   "/api",
			route:   "/users/",
			request: "/api/users/",
			want:    "/api/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()

			middlewares, err := NewMetrics(registry)
			require.NoError(t, err)

			sub := chi.NewRouter()
			sub.Get(tt.route, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			router := chi.NewRouter()
			router.Use(middlewares)
			router.Mount(tt.mount, sub)

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, tt.request, http.NoBody)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusOK, recorder.Code)

			resp, err := registry.Gather()
			require.NoError(t, err)

			counter := findMetricFamily(t, resp, "http_requests_total")
			require.Len(t, counter.GetMetric(), 1)
			require.Equal(t, tt.want, getValueForLabel(counter.GetMetric()[0].GetLabel(), "path"))
		})
	}
}

func findMetricFamily(tb testing.TB, families []*io_prometheus_client.MetricFamily, name string) *io_prometheus_client.MetricFamily {
	tb.Helper()

	for _, mf := range families {
		if mf.GetName() == name {
			return mf
		}
	}

	tb.Fatalf("metric family %q was not collected", name)

	return nil
}

func getValueForLabel(labels []*io_prometheus_client.LabelPair, labelName string) string {
	for _, l := range labels {
		if l.GetName() == labelName {
			return l.GetValue()
		}
	}

	return ""
}
