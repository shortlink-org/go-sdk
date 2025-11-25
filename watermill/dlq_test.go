package watermill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPublisher struct {
	mu        sync.Mutex
	published []publishedMessage
	err       error
}

type publishedMessage struct {
	topic string
	msg   *message.Message
}

func (s *stubPublisher) Publish(topic string, msgs ...*message.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		return s.err
	}

	for _, msg := range msgs {
		s.published = append(s.published, publishedMessage{topic: topic, msg: msg})
	}

	return nil
}

func (s *stubPublisher) Close() error { return nil }

func TestDLQMiddlewarePublishesAfterMaxRetries(t *testing.T) {
	pub := &stubPublisher{}
	mw := DLQMiddleware(pub, 1)

	handlerCalls := 0
	handler := mw(func(msg *message.Message) ([]*message.Message, error) {
		handlerCalls++
		return nil, errors.New("boom")
	})

	msg := message.NewMessage("msg-id", []byte(`{"key":"value"}`))
	msg.Metadata = map[string]string{
		"received_topic": "orders",
	}

	_, err := handler(msg)
	require.EqualError(t, err, "boom")
	assert.Len(t, pub.published, 0)

	_, err = handler(msg)
	require.NoError(t, err, "second attempt should route to DLQ and swallow error")
	require.Len(t, pub.published, 1)

	published := pub.published[0]
	assert.Equal(t, "orders.DLQ", published.topic)

	var dlqMsg DLQMessage
	require.NoError(t, json.Unmarshal(published.msg.Payload, &dlqMsg))

	assert.Equal(t, "orders", dlqMsg.Topic)
	assert.Equal(t, "boom", dlqMsg.Error)
	assert.Equal(t, msg.UUID, dlqMsg.OriginalUUID)
	assert.Equal(t, 1, dlqMsg.RetryCount)
	assert.Equal(t, map[string]string{
		"received_topic":    "orders",
		dlqRetryMetadataKey: "1",
	}, dlqMsg.Metadata)
	assert.JSONEq(t, `{"key":"value"}`, string(dlqMsg.Payload))
	assert.Equal(t, 2, handlerCalls)
}

func TestDLQMiddlewareMissingTopic(t *testing.T) {
	pub := &stubPublisher{}
	mw := DLQMiddleware(pub, 0)

	handler := mw(func(msg *message.Message) ([]*message.Message, error) {
		return nil, fmt.Errorf("fail")
	})

	msg := message.NewMessage("msg-id", []byte(`{"foo":"bar"}`))
	_, err := handler(msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing topic")
	assert.Len(t, pub.published, 0)
}

func TestDLQMiddlewarePublishErrorPropagated(t *testing.T) {
	pub := &stubPublisher{err: errors.New("publish failed")}
	mw := DLQMiddleware(pub, 0)

	handler := mw(func(msg *message.Message) ([]*message.Message, error) {
		return nil, errors.New("boom")
	})

	msg := message.NewMessage("msg-id", []byte(`{"foo":"bar"}`))
	msg.Metadata = map[string]string{
		"received_topic": "payments",
	}

	_, err := handler(msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dlq publish error")
}

func TestDLQMiddlewarePreservesContext(t *testing.T) {
	pub := &stubPublisher{}
	mw := DLQMiddleware(pub, 0)

	handler := mw(func(msg *message.Message) ([]*message.Message, error) {
		return nil, errors.New("boom")
	})

	ctx := context.WithValue(context.Background(), struct{}{}, "ctx")

	msg := message.NewMessage("msg-id", []byte(`{"foo":"bar"}`))
	msg.Metadata = map[string]string{
		"received_topic": "events",
	}
	msg.SetContext(ctx)

	_, err := handler(msg)
	require.NoError(t, err)

	require.Len(t, pub.published, 1)
	assert.Equal(t, ctx, pub.published[0].msg.Context())
}
