package bus

import (
	"context"
	"errors"
	"fmt"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
)

var (
	errCommandBusUninitialized = errors.New("cqrs/bus: command bus is not initialized")
	errCommandPublisherNil     = errors.New("cqrs/bus: publisher is required")
	errCommandMarshalerNil     = errors.New("cqrs/bus: marshaler is required")
	errCommandPayloadNil       = errors.New("cqrs/bus: command is nil")
)

// CommandBus publishes commands to underlying transport.
type CommandBus struct {
	publisher wmmessage.Publisher
	marshaler cqrsmessage.Marshaler
	namer     cqrsmessage.Namer
}

// NewCommandBus builds a bus backed by Watermill publisher.
func NewCommandBus(pub wmmessage.Publisher, marshaler cqrsmessage.Marshaler, namer cqrsmessage.Namer) *CommandBus {
	return &CommandBus{
		publisher: pub,
		marshaler: marshaler,
		namer:     namer,
	}
}

// validate checks that the command bus and its dependencies are properly initialized.
func (b *CommandBus) validate(cmd any) error {
	if b == nil {
		return errCommandBusUninitialized
	}
	if b.publisher == nil {
		return errCommandPublisherNil
	}
	if b.marshaler == nil {
		return errCommandMarshalerNil
	}
	if cmd == nil {
		return errCommandPayloadNil
	}
	return nil
}

// Send encodes and publishes command with Shortlink metadata and tracing context.
func (b *CommandBus) Send(ctx context.Context, cmd any) error {
	if err := b.validate(cmd); err != nil {
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
		name = b.namer.CommandName(cmd)
		topic = b.namer.TopicForCommand(name)
		service = b.namer.ServiceName()
		ctx = cqrsmessage.WithServiceName(ctx, service)
	} else {
		name = cqrsmessage.NameOf(cmd)
		topic = cqrsmessage.TopicForCommand(name)
	}

	msg, err := b.marshaler.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal command %T: %w", cmd, err)
	}

	if msg.Metadata.Get(cqrsmessage.MetadataServiceName) == "" && service != "" {
		msg.Metadata.Set(cqrsmessage.MetadataServiceName, service)
	}
	msg.Metadata.Set(cqrsmessage.MetadataMessageKind, string(cqrsmessage.KindCommand))

	cqrsmessage.SetTrace(ctx, msg)

	msg.SetContext(ctx)

	return b.publisher.Publish(topic, msg)
}
