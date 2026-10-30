// Code generated from the go-sdk semantic convention registry. DO NOT EDIT.
//
// Source: semconv/registry/eventsourcing.yaml. Regenerate with `make -C semconv generate`.

package semconv

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// ConcurrencyConflictsTotal is the
// "shortlink_eventsourcing_concurrency_conflicts_total" instrument. Saves
// rejected because the aggregate moved past the expected version.
type ConcurrencyConflictsTotal struct {
	metric.Int64Counter
}

var newConcurrencyConflictsTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Saves rejected because the aggregate moved past the expected version."),
}

// NewConcurrencyConflictsTotal returns a new ConcurrencyConflictsTotal
// instrument created on m. A nil m yields a no-op instrument, which is what
// callers without observability get.
func NewConcurrencyConflictsTotal(m metric.Meter, opt ...metric.Int64CounterOption) (ConcurrencyConflictsTotal, error) {
	if m == nil {
		return ConcurrencyConflictsTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newConcurrencyConflictsTotalOpts
	} else {
		opt = append(opt, newConcurrencyConflictsTotalOpts...)
	}

	i, err := m.Int64Counter("shortlink_eventsourcing_concurrency_conflicts_total", opt...)
	if err != nil {
		return ConcurrencyConflictsTotal{noop.Int64Counter{}}, err
	}

	return ConcurrencyConflictsTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ConcurrencyConflictsTotal) Name() string {
	return "shortlink_eventsourcing_concurrency_conflicts_total"
}

// Unit returns the semantic convention unit of the instrument.
func (ConcurrencyConflictsTotal) Unit() string {
	return ""
}

// Description returns the semantic convention description of the instrument.
func (ConcurrencyConflictsTotal) Description() string {
	return "Saves rejected because the aggregate moved past the expected version."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// aggregateType: The type of the aggregate.
func (m ConcurrencyConflictsTotal) Add(
	ctx context.Context,
	incr int64,
	aggregateType string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		AggregateType(aggregateType),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// EventsAppendedTotal is the "shortlink_eventsourcing_events_appended_total"
// instrument. Events written to the event store.
type EventsAppendedTotal struct {
	metric.Int64Counter
}

var newEventsAppendedTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Events written to the event store."),
}

// NewEventsAppendedTotal returns a new EventsAppendedTotal instrument created on
// m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewEventsAppendedTotal(m metric.Meter, opt ...metric.Int64CounterOption) (EventsAppendedTotal, error) {
	if m == nil {
		return EventsAppendedTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newEventsAppendedTotalOpts
	} else {
		opt = append(opt, newEventsAppendedTotalOpts...)
	}

	i, err := m.Int64Counter("shortlink_eventsourcing_events_appended_total", opt...)
	if err != nil {
		return EventsAppendedTotal{noop.Int64Counter{}}, err
	}

	return EventsAppendedTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (EventsAppendedTotal) Name() string {
	return "shortlink_eventsourcing_events_appended_total"
}

// Unit returns the semantic convention unit of the instrument.
func (EventsAppendedTotal) Unit() string {
	return ""
}

// Description returns the semantic convention description of the instrument.
func (EventsAppendedTotal) Description() string {
	return "Events written to the event store."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// aggregateType: The type of the aggregate.
func (m EventsAppendedTotal) Add(
	ctx context.Context,
	incr int64,
	aggregateType string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		AggregateType(aggregateType),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}
