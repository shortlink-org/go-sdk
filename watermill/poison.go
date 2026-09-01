package watermill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
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

// NewShortlinkPoisonMiddleware adapts Watermill's poison queue to Shortlink DLQ builder.
//
// Order matters: add this middleware BEFORE the retry middleware, so that it
// wraps retry rather than sits under it. The poison queue publishes the dead
// letter and then reports success to whatever wraps it; underneath retry that
// success is all retry ever sees, and no message is retried — the first
// failure dead-letters it. Watermill executes the middleware added first as
// the outermost one, so "before retry" means literally the earlier
// AddMiddleware call:
//
//	router.AddMiddleware(watermill.NewShortlinkPoisonMiddleware(pub, "shortlink.dlq"))
//	router.AddMiddleware(retryMiddleware.Middleware)
//
// Clients built by New get this order for free — see WithPoisonQueue.
func NewShortlinkPoisonMiddleware(publisher message.Publisher, dlqTopic string) message.HandlerMiddleware {
	mw, err := newPoisonMiddleware(publisher, dlqTopic, "", nil)
	if err != nil {
		panic(fmt.Sprintf("watermill: %v", err))
	}

	return mw
}

// newPoisonMiddleware builds the poison middleware. A nil filter dead-letters
// every failure; an empty serviceName defers to SERVICE_NAME, read at each
// publication rather than cached, so a name set after start-up still lands on
// the DLQ event.
func newPoisonMiddleware(
	publisher message.Publisher,
	dlqTopic, serviceName string,
	shouldGoToPoisonQueue func(err error) bool,
) (message.HandlerMiddleware, error) {
	if publisher == nil {
		return nil, ErrPoisonPublisherRequired
	}

	wrappedPublisher := &poisonPublisher{
		topic:       dlqTopic,
		publisher:   publisher,
		serviceName: serviceName,
	}

	poisonTopic := dlqTopic
	if poisonTopic == "" {
		poisonTopic = defaultDLQTopic
	}

	var (
		poisonMW message.HandlerMiddleware
		err      error
	)

	if shouldGoToPoisonQueue == nil {
		poisonMW, err = middleware.PoisonQueue(wrappedPublisher, poisonTopic)
	} else {
		poisonMW, err = middleware.PoisonQueueWithFilter(wrappedPublisher, poisonTopic, shouldGoToPoisonQueue)
	}

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

// poisonTopicDescription names the configured DLQ topic for logs, spelling out
// the per-topic default that an empty configuration means.
func poisonTopicDescription(dlqTopic string) string {
	if dlqTopic == "" {
		return "<received_topic>.DLQ"
	}

	return dlqTopic
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
			ServiceName: p.currentServiceName(),
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

// currentServiceName resolves the name stamped on a DLQ event. The environment
// is read on every publication on purpose: caching it in a sync.Once pinned
// the very first read for the life of the process, so a SERVICE_NAME exported
// after the first dead letter — or after the first test touched the package —
// left every later event attributed to "unknown-service".
func (p *poisonPublisher) currentServiceName() string {
	if p.serviceName != "" {
		return p.serviceName
	}

	name := strings.TrimSpace(os.Getenv("SERVICE_NAME"))
	if name == "" {
		return "unknown-service"
	}

	return name
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
//
// It carries the same ordering requirement as NewShortlinkPoisonMiddleware:
// add it BEFORE the retry middleware, or retry never sees a failure and never
// retries anything.
func NewShortlinkPoisonMiddlewareWithFilter(
	publisher message.Publisher,
	dlqTopic string,
	shouldGoToPoisonQueue func(err error) bool,
) (message.HandlerMiddleware, error) {
	return newPoisonMiddleware(publisher, dlqTopic, "", shouldGoToPoisonQueue)
}
