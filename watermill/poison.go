package watermill

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/shortlink-org/go-sdk/watermill/dlq"
)

type originalMessageCtxKey struct{}

var (
	serviceNameOnce sync.Once
	cachedService   string
)

// NewShortlinkPoisonMiddleware adapts Watermill's poison queue to Shortlink DLQ builder.
func NewShortlinkPoisonMiddleware(publisher message.Publisher, dlqTopic string) message.HandlerMiddleware {
	if publisher == nil {
		panic("watermill: poison middleware requires a publisher")
	}

	wrappedPublisher := &poisonPublisher{
		topic:       dlqTopic,
		publisher:   publisher,
		serviceName: detectServiceName(),
	}

	poisonTopic := dlqTopic
	if poisonTopic == "" {
		poisonTopic = "shortlink.dlq"
	}

	poisonMW, err := middleware.PoisonQueue(wrappedPublisher, poisonTopic)
	if err != nil {
		panic(fmt.Sprintf("watermill: poison middleware init failed: %v", err))
	}

	return func(h message.HandlerFunc) message.HandlerFunc {
		return poisonMW(func(msg *message.Message) ([]*message.Message, error) {
			ctx := ensureContext(msg.Context())
			ctx = context.WithValue(ctx, originalMessageCtxKey{}, snapshotMessage(msg))
			msg.SetContext(ctx)
			return h(msg)
		})
	}
}

func detectServiceName() string {
	serviceNameOnce.Do(func() {
		cachedService = os.Getenv("SERVICE_NAME")
		if cachedService == "" {
			cachedService = "unknown-service"
		}
	})

	return cachedService
}

func ensureContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}

	return context.Background()
}

func snapshotMessage(msg *message.Message) *message.Message {
	cloned := message.NewMessage(msg.UUID, append([]byte(nil), msg.Payload...))
	for k, v := range msg.Metadata {
		cloned.Metadata.Set(k, v)
	}

	cloned.SetContext(msg.Context())

	return cloned
}

type poisonPublisher struct {
	topic       string
	publisher   message.Publisher
	serviceName string
}

func (p *poisonPublisher) Publish(_ string, msgs ...*message.Message) error {
	for _, poisoned := range msgs {
		ctx := ensureContext(poisoned.Context())

		original, _ := ctx.Value(originalMessageCtxKey{}).(*message.Message)
		if original == nil {
			original = snapshotMessage(poisoned)
		}

		targetTopic, err := p.resolveTopic(poisoned)
		if err != nil {
			return err
		}

		event := dlq.DLQEvent{
			FailedAt:    time.Now().UTC(),
			Reason:      poisoned.Metadata.Get(middleware.ReasonForPoisonedKey),
			OriginalMsg: original,
			Stacktrace:  string(debug.Stack()),
			ServiceName: p.serviceName,
		}

		if event.Reason == "" {
			event.Reason = "handler returned error"
		}

		if err := dlq.PublishDLQ(ctx, p.publisher, targetTopic, event); err != nil {
			return err
		}
	}

	return nil
}

func (p *poisonPublisher) Close() error {
	return p.publisher.Close()
}

func (p *poisonPublisher) resolveTopic(msg *message.Message) (string, error) {
	if p.topic != "" {
		return p.topic, nil
	}

	topic := msg.Metadata.Get("received_topic")
	if topic == "" {
		topic = msg.Metadata.Get("topic")
	}
	if topic == "" {
		return "", fmt.Errorf("missing topic metadata for DLQ publication")
	}

	return topic + ".DLQ", nil
}
