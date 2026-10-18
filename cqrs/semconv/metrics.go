// Code generated from the go-sdk semantic convention registry. DO NOT EDIT.
//
// Source: semconv/registry/cqrs.yaml. Regenerate with `make -C semconv generate`.

package semconv

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// OutboxFailedTotal is the "shortlink_cqrs_outbox_failed_total" instrument.
// Outbox messages the forwarder failed to deliver.
type OutboxFailedTotal struct {
	metric.Int64Counter
}

var newOutboxFailedTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Outbox messages the forwarder failed to deliver."),
}

// NewOutboxFailedTotal returns a new OutboxFailedTotal instrument created on m.
// A nil m yields a no-op instrument, which is what callers without observability
// get.
func NewOutboxFailedTotal(m metric.Meter, opt ...metric.Int64CounterOption) (OutboxFailedTotal, error) {
	if m == nil {
		return OutboxFailedTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newOutboxFailedTotalOpts
	} else {
		opt = append(opt, newOutboxFailedTotalOpts...)
	}

	i, err := m.Int64Counter("shortlink_cqrs_outbox_failed_total", opt...)
	if err != nil {
		return OutboxFailedTotal{noop.Int64Counter{}}, err
	}

	return OutboxFailedTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (OutboxFailedTotal) Name() string {
	return "shortlink_cqrs_outbox_failed_total"
}

// Unit returns the semantic convention unit of the instrument.
func (OutboxFailedTotal) Unit() string {
	return ""
}

// Description returns the semantic convention description of the instrument.
func (OutboxFailedTotal) Description() string {
	return "Outbox messages the forwarder failed to deliver."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// forwarderName: The name the outbox forwarder was registered under.
func (m OutboxFailedTotal) Add(
	ctx context.Context,
	incr int64,
	forwarderName string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		ForwarderName(forwarderName),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// OutboxForwardedTotal is the "shortlink_cqrs_outbox_forwarded_total"
// instrument. Outbox messages the forwarder delivered to the real publisher.
type OutboxForwardedTotal struct {
	metric.Int64Counter
}

var newOutboxForwardedTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Outbox messages the forwarder delivered to the real publisher."),
}

// NewOutboxForwardedTotal returns a new OutboxForwardedTotal instrument created
// on m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewOutboxForwardedTotal(m metric.Meter, opt ...metric.Int64CounterOption) (OutboxForwardedTotal, error) {
	if m == nil {
		return OutboxForwardedTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newOutboxForwardedTotalOpts
	} else {
		opt = append(opt, newOutboxForwardedTotalOpts...)
	}

	i, err := m.Int64Counter("shortlink_cqrs_outbox_forwarded_total", opt...)
	if err != nil {
		return OutboxForwardedTotal{noop.Int64Counter{}}, err
	}

	return OutboxForwardedTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (OutboxForwardedTotal) Name() string {
	return "shortlink_cqrs_outbox_forwarded_total"
}

// Unit returns the semantic convention unit of the instrument.
func (OutboxForwardedTotal) Unit() string {
	return ""
}

// Description returns the semantic convention description of the instrument.
func (OutboxForwardedTotal) Description() string {
	return "Outbox messages the forwarder delivered to the real publisher."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// forwarderName: The name the outbox forwarder was registered under.
func (m OutboxForwardedTotal) Add(
	ctx context.Context,
	incr int64,
	forwarderName string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		ForwarderName(forwarderName),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}
