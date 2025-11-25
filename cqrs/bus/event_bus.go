package bus

import (
	"context"
	"errors"
	"fmt"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
)

var (
	errEventBusUninitialized = errors.New("cqrs/bus: event bus is not initialized")
	errEventPublisherNil     = errors.New("cqrs/bus: publisher is required")
	errEventMarshalerNil     = errors.New("cqrs/bus: marshaler is required")
	errEventPayloadNil       = errors.New("cqrs/bus: event is nil")
)

// EventBus publishes domain events.
type EventBus struct {
	publisher wmmessage.Publisher
	marshaler cqrsmessage.Marshaler
	namer     cqrsmessage.Namer
}

// NewEventBus builds EventBus with required dependencies.
func NewEventBus(pub wmmessage.Publisher, marshaler cqrsmessage.Marshaler, namer cqrsmessage.Namer) *EventBus {
	return &EventBus{
		publisher: pub,
		marshaler: marshaler,
		namer:     namer,
	}
}

// validate checks that the event bus and its dependencies are properly initialized.
func (b *EventBus) validate(evt any) error {
	if b == nil {
		return errEventBusUninitialized
	}
	if b.publisher == nil {
		return errEventPublisherNil
	}
	if b.marshaler == nil {
		return errEventMarshalerNil
	}
	if evt == nil {
		return errEventPayloadNil
	}
	return nil
}

// Publish sends event using canonical topic name.
func (b *EventBus) Publish(ctx context.Context, evt any) error {
	if err := b.validate(evt); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var (
		name    string
		topic   string
		service string
	)

	if b.namer != nil {
		name = b.namer.EventName(evt)
		topic = b.namer.TopicForEvent(name)
		service = b.namer.ServiceName()
		ctx = cqrsmessage.WithServiceName(ctx, service)
	} else {
		name = cqrsmessage.NameOf(evt)
		topic = cqrsmessage.TopicForEvent(name)
	}

	msg, err := b.marshaler.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event %T: %w", evt, err)
	}

	if msg.Metadata.Get(cqrsmessage.MetadataServiceName) == "" && service != "" {
		msg.Metadata.Set(cqrsmessage.MetadataServiceName, service)
	}
	msg.Metadata.Set(cqrsmessage.MetadataMessageKind, string(cqrsmessage.KindEvent))

	cqrsmessage.SetTrace(ctx, msg)

	msg.SetContext(ctx)

	return b.publisher.Publish(topic, msg)
}
