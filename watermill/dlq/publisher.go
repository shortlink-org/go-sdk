package dlq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

//nolint:gochecknoglobals // package-level default, mirroring slog.Default()
var (
	logMu     sync.RWMutex
	pkgLogger = slog.Default()
)

// SetLogger plugs a service's configured logger into the DLQ helpers. A nil
// logger restores the default rather than being ignored.
func SetLogger(log *slog.Logger) {
	logMu.Lock()
	defer logMu.Unlock()

	if log == nil {
		pkgLogger = slog.Default()

		return
	}

	pkgLogger = log
}

// Logger reports the logger currently used by the DLQ helpers.
func Logger() *slog.Logger {
	logMu.RLock()
	defer logMu.RUnlock()

	return pkgLogger
}

// Preconditions PublishDLQ refuses to guess at.
var (
	// ErrNilPublisher reports a DLQ publish with no publisher to send to.
	ErrNilPublisher = errors.New("dlq publisher is nil")
	// ErrEmptyTopic reports a DLQ publish with no topic to send to.
	ErrEmptyTopic = errors.New("dlq topic is empty")
)

// PublishDLQ builds the DLQ message and forwards it using the provided publisher.
//
//nolint:contextcheck,gocritic // the caller's context is passed through; DLQEvent is public API
func PublishDLQ(ctx context.Context, publisher message.Publisher, topic string, event DLQEvent) error {
	if publisher == nil {
		return ErrNilPublisher
	}

	if topic == "" {
		return ErrEmptyTopic
	}

	msg, err := BuildDLQMessage(event)
	if err != nil {
		return fmt.Errorf("build dlq message: %w", err)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	msg.SetContext(ctx)
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(msg.Metadata))

	log := Logger()
	log.DebugContext(ctx, "Publishing DLQ message",
		slog.String("topic", topic),
		slog.String("reason", event.Reason),
		slog.String("message_id", msg.UUID),
	)

	err = publisher.Publish(topic, msg)
	if err != nil {
		log.ErrorContext(ctx, "Failed to publish DLQ message",
			slog.String("topic", topic),
			slog.String("reason", event.Reason),
			slog.String("message_id", msg.UUID),
			slog.String("error", err.Error()),
		)

		return fmt.Errorf("publish dlq message: %w", err)
	}

	log.InfoContext(ctx, "Published DLQ message",
		slog.String("topic", topic),
		slog.String("reason", event.Reason),
		slog.String("message_id", msg.UUID),
	)

	return nil
}
