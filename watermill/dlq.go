package watermill

import (
	"context"

	"github.com/ThreeDotsLabs/watermill/message"
	shortdlq "github.com/shortlink-org/go-sdk/watermill/dlq"
)

// DLQEvent is an alias for the Shortlink DLQ event structure.
type DLQEvent = shortdlq.DLQEvent

// PublishDLQ publishes the provided DLQ event using Shortlink's DLQ helpers.
func PublishDLQ(ctx context.Context, publisher message.Publisher, topic string, event DLQEvent) error {
	return shortdlq.PublishDLQ(ctx, publisher, topic, event)
}
