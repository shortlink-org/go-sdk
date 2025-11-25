package dlq

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

var (
	logOnce   sync.Once
	logMu     sync.RWMutex
	pkgLogger logger.Logger
)

func getLogger() logger.Logger {
	logOnce.Do(func() {
		l, err := logger.New(logger.Default())
		if err != nil {
			panic(fmt.Sprintf("dlq logger init failed: %v", err))
		}
		logMu.Lock()
		pkgLogger = l
		logMu.Unlock()
	})

	logMu.RLock()
	defer logMu.RUnlock()

	return pkgLogger
}

// SetLogger allows services to plug their configured logger into DLQ helpers.
func SetLogger(log logger.Logger) {
	if log == nil {
		return
	}

	logMu.Lock()
	pkgLogger = log
	logMu.Unlock()
}

// Logger exposes the logger currently used by the DLQ helpers.
func Logger() logger.Logger {
	return getLogger()
}

// PublishDLQ builds the DLQ message and forwards it using the provided publisher.
func PublishDLQ(ctx context.Context, publisher message.Publisher, topic string, event DLQEvent) error {
	if publisher == nil {
		return fmt.Errorf("dlq publisher is nil")
	}

	if topic == "" {
		return fmt.Errorf("dlq topic is empty")
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
	log.DebugWithContext(ctx, "Publishing DLQ message",
		slog.String("topic", topic),
		slog.String("reason", event.Reason),
		slog.String("message_id", msg.UUID),
	)

	if err := publisher.Publish(topic, msg); err != nil {
		log.ErrorWithContext(ctx, "Failed to publish DLQ message",
			slog.String("topic", topic),
			slog.String("reason", event.Reason),
			slog.String("message_id", msg.UUID),
			slog.String("error", err.Error()),
		)

		return fmt.Errorf("publish dlq message: %w", err)
	}

	log.InfoWithContext(ctx, "Published DLQ message",
		slog.String("topic", topic),
		slog.String("reason", event.Reason),
		slog.String("message_id", msg.UUID),
	)

	return nil
}
