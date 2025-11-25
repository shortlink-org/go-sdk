package dlq

import (
	"context"
	"sync"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type testPublisher struct {
	mu     sync.Mutex
	topics []string
	msgs   []*message.Message
}

func (p *testPublisher) Publish(topic string, msgs ...*message.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, msg := range msgs {
		p.topics = append(p.topics, topic)
		p.msgs = append(p.msgs, msg)
	}

	return nil
}

func (p *testPublisher) Close() error { return nil }

func TestPublishDLQInjectsTraceContext(t *testing.T) {
	originalPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTextMapPropagator(originalPropagator)
	})

	pub := &testPublisher{}
	original := message.NewMessage("source", []byte(`{"foo":"bar"}`))

	traceID, _ := trace.TraceIDFromHex("463ac35c9f6413ad48485a3953bb6124")
	spanID, _ := trace.SpanIDFromHex("0020000000000001")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	event := DLQEvent{Reason: "boom", OriginalMsg: original}

	err := PublishDLQ(ctx, pub, "dlq-topic", event)
	require.NoError(t, err)

	require.Len(t, pub.msgs, 1)
	msg := pub.msgs[0]

	traceparent := msg.Metadata.Get("traceparent")
	require.NotEmpty(t, traceparent)
	require.Contains(t, traceparent, traceID.String())
}
