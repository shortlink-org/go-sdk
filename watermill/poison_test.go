package watermill

import (
	"errors"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/require"
)

type poisonTestPublisher struct {
	published []publishedMessage
}

type publishedMessage struct {
	topic string
	msg   *message.Message
}

func (p *poisonTestPublisher) Publish(topic string, msgs ...*message.Message) error {
	for _, msg := range msgs {
		p.published = append(p.published, publishedMessage{topic: topic, msg: msg})
	}
	return nil
}

func (p *poisonTestPublisher) Close() error { return nil }

func TestShortlinkPoisonMiddlewarePublishesToDLQ(t *testing.T) {
	pub := &poisonTestPublisher{}
	mw := NewShortlinkPoisonMiddleware(pub, "dlq.topic")

	handler := mw(func(msg *message.Message) ([]*message.Message, error) {
		return nil, errors.New("boom")
	})

	msg := message.NewMessage("msg-id", []byte(`{"foo":"bar"}`))
	msg.Metadata.Set("received_topic", "orders")

	produced, err := handler(msg)
	require.NoError(t, err)
	require.Nil(t, produced)

	require.Len(t, pub.published, 1)
	published := pub.published[0]
	require.Equal(t, "dlq.topic", published.topic)
	require.Equal(t, "boom", published.msg.Metadata.Get("poison_reason"))
	require.Equal(t, "orders", published.msg.Metadata.Get("original_received_topic"))
}

func TestShortlinkPoisonMiddlewareDerivesTopicWhenEmpty(t *testing.T) {
	pub := &poisonTestPublisher{}
	mw := NewShortlinkPoisonMiddleware(pub, "")

	handler := mw(func(msg *message.Message) ([]*message.Message, error) {
		return nil, errors.New("fail")
	})

	msg := message.NewMessage("msg-id", []byte("test"))
	msg.Metadata.Set("received_topic", "payments")

	_, err := handler(msg)
	require.NoError(t, err)

	require.Len(t, pub.published, 1)
	require.Equal(t, "payments.DLQ", pub.published[0].topic)
}
