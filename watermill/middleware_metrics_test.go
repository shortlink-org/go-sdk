//go:build unit

package watermill

import (
	"context"
	"errors"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/shortlink-org/go-sdk/watermill/semconv"
)

// nopPublisher accepts every publish.
type nopPublisher struct{}

func (nopPublisher) Publish(string, ...*message.Message) error { return nil }
func (nopPublisher) Close() error                              { return nil }

// errPublisher fails every publish with the error it was built with.
type errPublisher struct{ err error }

func (p errPublisher) Publish(string, ...*message.Message) error { return p.err }
func (p errPublisher) Close() error                              { return nil }

// TestMetricsMiddleware_EmitsRegistrySignals pins the metric names and the
// attributes the module emits to the ones declared in
// semconv/registry/watermill.yaml. A change on either side has to show up here.
func TestMetricsMiddleware_EmitsRegistrySignals(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	metrics, err := NewMetricsMiddleware(nil, provider)
	require.NoError(t, err)

	// Publish: one success of two messages, one failure.
	msg := func() *message.Message {
		m := message.NewMessage("id", nil)
		m.SetContext(context.Background())

		return m
	}

	okPub := metrics.PublisherWrapper(nopPublisher{}, nil)
	require.NoError(t, okPub.Publish("orders", msg(), msg()))

	badPub := metrics.PublisherWrapper(errPublisher{err: errors.New("broker down")}, nil)
	require.Error(t, badPub.Publish("orders", msg()))

	// Consume: one success, one failure.
	handler := metrics.HandlerMiddleware()

	consumed := msg()
	consumed.Metadata.Set("received_topic", "orders")

	_, err = handler(func(*message.Message) ([]*message.Message, error) { return nil, nil })(consumed)
	require.NoError(t, err)

	_, err = handler(func(*message.Message) ([]*message.Message, error) {
		return nil, errors.New("handler failed")
	})(consumed)
	require.Error(t, err)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	byName := map[string]metricdata.Metrics{}

	for _, scope := range rm.ScopeMetrics {
		require.Equal(t, "watermill", scope.Scope.Name)

		for _, m := range scope.Metrics {
			byName[m.Name] = m
		}
	}

	// The names are the public contract of the module; spell them out rather
	// than read them back from the generated code.
	want := map[string]string{
		"watermill_messages_published_total": semconv.MessagesPublishedTotal{}.Name(),
		"watermill_messages_consumed_total":  semconv.MessagesConsumedTotal{}.Name(),
		"watermill_messages_failed_total":    semconv.MessagesFailedTotal{}.Name(),
		"watermill_publish_latency_seconds":  semconv.PublishLatencySeconds{}.Name(),
		"watermill_consume_latency_seconds":  semconv.ConsumeLatencySeconds{}.Name(),
	}

	for name, generated := range want {
		require.Equal(t, name, generated, "generated code drifted from the contract")
		require.Contains(t, byName, name)
	}

	require.Len(t, byName, len(want), "an instrument the registry does not declare was emitted")

	published, ok := byName["watermill_messages_published_total"].Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, published.DataPoints, 1)
	require.Equal(t, int64(2), published.DataPoints[0].Value)
	requireAttrs(t, published.DataPoints[0].Attributes, semconv.Topic("orders"))

	failed, ok := byName["watermill_messages_failed_total"].Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, failed.DataPoints, 2)

	for _, dp := range failed.DataPoints {
		stage, found := dp.Attributes.Value(semconv.StageKey)
		require.True(t, found)

		switch semconv.StageAttr(stage.AsString()) {
		case semconv.StagePublish:
			requireAttrs(t, dp.Attributes, semconv.Topic("orders"), semconv.Error("broker down"))
		case semconv.StageConsume:
			requireAttrs(t, dp.Attributes, semconv.Topic("orders"), semconv.Error("handler failed"))
		default:
			t.Fatalf("unexpected stage %q", stage.AsString())
		}
	}

	require.Equal(t, "s", byName["watermill_publish_latency_seconds"].Unit)
	require.Equal(t, "s", byName["watermill_consume_latency_seconds"].Unit)
}

// TestMetricsMiddleware_NoSpanNoTraceAttributes checks that trace_id and
// span_id stay off the series when the context carries no span.
func TestMetricsMiddleware_NoSpanNoTraceAttributes(t *testing.T) {
	t.Parallel()

	attrs := spanAttributes(context.Background())
	require.Empty(t, attrs)
}

func requireAttrs(t *testing.T, set attribute.Set, want ...attribute.KeyValue) {
	t.Helper()

	for _, kv := range want {
		got, ok := set.Value(kv.Key)
		require.True(t, ok, "attribute %q missing", kv.Key)
		require.Equal(t, kv.Value.AsInterface(), got.AsInterface(), "attribute %q", kv.Key)
	}
}
