package watermill

import (
	"context"
	"errors"
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

// ErrMissingTopic reports a poisoned message that does not say where it came
// from, so there is no DLQ topic to derive.
var ErrMissingTopic = errors.New("missing topic metadata for DLQ publication")

// defaultDLQTopic is where poisoned messages go when no topic is configured.
const defaultDLQTopic = "shortlink.dlq"

// ErrPoisonPublisherRequired reports a poison middleware built without a
// publisher to send dead letters to.
var ErrPoisonPublisherRequired = errors.New("watermill: poison middleware requires a publisher")

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
		poisonTopic = defaultDLQTopic
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

//nolint:errcheck // cleanup path: there is nothing useful to do with the error
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

		err = dlq.PublishDLQ(ctx, p.publisher, targetTopic, event)
		if err != nil {
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
		return "", ErrMissingTopic
	}

	return topic + ".DLQ", nil
}

// NewShortlinkPoisonMiddlewareWithFilter is NewShortlinkPoisonMiddleware with
// a say in what counts as poison.
//
// The filter reports whether a failure should send the message to the DLQ.
// Return false for failures that are transient by construction — a message
// that arrived before the read replica had replayed its writes is the
// motivating case: it is not malformed, it is early, and dead-lettering it
// turns a consistency feature into a source of dead letters.
//
//	NewShortlinkPoisonMiddlewareWithFilter(pub, topic, func(err error) bool {
//		return !IsConsistencyError(err)
//	})
func NewShortlinkPoisonMiddlewareWithFilter(
	publisher message.Publisher,
	dlqTopic string,
	shouldGoToPoisonQueue func(err error) bool,
) (message.HandlerMiddleware, error) {
	if publisher == nil {
		return nil, ErrPoisonPublisherRequired
	}

	if shouldGoToPoisonQueue == nil {
		shouldGoToPoisonQueue = func(error) bool { return true }
	}

	wrappedPublisher := &poisonPublisher{
		topic:       dlqTopic,
		publisher:   publisher,
		serviceName: detectServiceName(),
	}

	poisonTopic := dlqTopic
	if poisonTopic == "" {
		poisonTopic = defaultDLQTopic
	}

	poisonMW, err := middleware.PoisonQueueWithFilter(wrappedPublisher, poisonTopic, shouldGoToPoisonQueue)
	if err != nil {
		return nil, fmt.Errorf("watermill: poison middleware init failed: %w", err)
	}

	return func(h message.HandlerFunc) message.HandlerFunc {
		return poisonMW(func(msg *message.Message) ([]*message.Message, error) {
			ctx := ensureContext(msg.Context())
			ctx = context.WithValue(ctx, originalMessageCtxKey{}, snapshotMessage(msg))
			msg.SetContext(ctx)

			return h(msg)
		})
	}, nil
}
