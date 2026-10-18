// Code generated from the go-sdk semantic convention registry. DO NOT EDIT.
//
// Source: semconv/registry/watermill.yaml. Regenerate with `make -C semconv generate`.

package semconv

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// ConsistencyGateResultsTotal is the "watermill_consistency_gate_results_total"
// instrument. Outcomes of waiting for a read replica to replay a message's
// watermark.
//
// `outcome` is one of `ready` (the replica caught up), `not_caught_up` (the wait
// expired and the message is retried), `gate_error` (the gate failed) or
// `no_watermark` (the message carried none).
type ConsistencyGateResultsTotal struct {
	metric.Int64Counter
}

var newConsistencyGateResultsTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Outcomes of waiting for a read replica to replay a message's watermark."),
	metric.WithUnit("1"),
}

// NewConsistencyGateResultsTotal returns a new ConsistencyGateResultsTotal
// instrument created on m. A nil m yields a no-op instrument, which is what
// callers without observability get.
func NewConsistencyGateResultsTotal(m metric.Meter, opt ...metric.Int64CounterOption) (ConsistencyGateResultsTotal, error) {
	if m == nil {
		return ConsistencyGateResultsTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newConsistencyGateResultsTotalOpts
	} else {
		opt = append(opt, newConsistencyGateResultsTotalOpts...)
	}

	i, err := m.Int64Counter("watermill_consistency_gate_results_total", opt...)
	if err != nil {
		return ConsistencyGateResultsTotal{noop.Int64Counter{}}, err
	}

	return ConsistencyGateResultsTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ConsistencyGateResultsTotal) Name() string {
	return "watermill_consistency_gate_results_total"
}

// Unit returns the semantic convention unit of the instrument.
func (ConsistencyGateResultsTotal) Unit() string {
	return "1"
}

// Description returns the semantic convention description of the instrument.
func (ConsistencyGateResultsTotal) Description() string {
	return "Outcomes of waiting for a read replica to replay a message's watermark."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// outcome: How the operation ended. The set of values is listed on each metric
// that records it.
func (m ConsistencyGateResultsTotal) Add(
	ctx context.Context,
	incr int64,
	outcome string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		Outcome(outcome),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// ConsistencyStampsTotal is the "watermill_consistency_stamps_total" instrument.
// Messages stamped with a database watermark by the consistency publisher.
//
// `outcome` is one of `stamped` (a watermark was written), `nothing_written`
// (the handler made no writes), `unresolved` (a write happened but its position
// could not be captured) or `error` (the watermarker failed).
type ConsistencyStampsTotal struct {
	metric.Int64Counter
}

var newConsistencyStampsTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Messages stamped with a database watermark by the consistency publisher."),
	metric.WithUnit("1"),
}

// NewConsistencyStampsTotal returns a new ConsistencyStampsTotal instrument
// created on m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewConsistencyStampsTotal(m metric.Meter, opt ...metric.Int64CounterOption) (ConsistencyStampsTotal, error) {
	if m == nil {
		return ConsistencyStampsTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newConsistencyStampsTotalOpts
	} else {
		opt = append(opt, newConsistencyStampsTotalOpts...)
	}

	i, err := m.Int64Counter("watermill_consistency_stamps_total", opt...)
	if err != nil {
		return ConsistencyStampsTotal{noop.Int64Counter{}}, err
	}

	return ConsistencyStampsTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ConsistencyStampsTotal) Name() string {
	return "watermill_consistency_stamps_total"
}

// Unit returns the semantic convention unit of the instrument.
func (ConsistencyStampsTotal) Unit() string {
	return "1"
}

// Description returns the semantic convention description of the instrument.
func (ConsistencyStampsTotal) Description() string {
	return "Messages stamped with a database watermark by the consistency publisher."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// outcome: How the operation ended. The set of values is listed on each metric
// that records it.
func (m ConsistencyStampsTotal) Add(
	ctx context.Context,
	incr int64,
	outcome string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		Outcome(outcome),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// ConsumeLatencySeconds is the "watermill_consume_latency_seconds" instrument.
// Duration of one successful handler invocation.
type ConsumeLatencySeconds struct {
	metric.Float64Histogram
}

var newConsumeLatencySecondsOpts = []metric.Float64HistogramOption{
	metric.WithDescription("Duration of one successful handler invocation."),
	metric.WithUnit("s"),
}

// NewConsumeLatencySeconds returns a new ConsumeLatencySeconds instrument
// created on m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewConsumeLatencySeconds(m metric.Meter, opt ...metric.Float64HistogramOption) (ConsumeLatencySeconds, error) {
	if m == nil {
		return ConsumeLatencySeconds{noop.Float64Histogram{}}, nil
	}

	if len(opt) == 0 {
		opt = newConsumeLatencySecondsOpts
	} else {
		opt = append(opt, newConsumeLatencySecondsOpts...)
	}

	i, err := m.Float64Histogram("watermill_consume_latency_seconds", opt...)
	if err != nil {
		return ConsumeLatencySeconds{noop.Float64Histogram{}}, err
	}

	return ConsumeLatencySeconds{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ConsumeLatencySeconds) Name() string {
	return "watermill_consume_latency_seconds"
}

// Unit returns the semantic convention unit of the instrument.
func (ConsumeLatencySeconds) Unit() string {
	return "s"
}

// Description returns the semantic convention description of the instrument.
func (ConsumeLatencySeconds) Description() string {
	return "Duration of one successful handler invocation."
}

// Record records val to the current distribution for the given attributes.
// Required attributes are positional; attrs carries the optional ones.
//
// topic: The Watermill topic the message was published to or consumed from.
//
// Optional attributes: span_id, trace_id.
func (m ConsumeLatencySeconds) Record(
	ctx context.Context,
	val float64,
	topic string,
	attrs ...attribute.KeyValue,
) {
	if !m.Float64Histogram.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		Topic(topic),
	)
	all = append(all, attrs...)

	m.Float64Histogram.Record(ctx, val, metric.WithAttributes(all...))
}

// MessagesConsumedTotal is the "watermill_messages_consumed_total" instrument.
// Messages consumed from topics whose handler returned no error.
type MessagesConsumedTotal struct {
	metric.Int64Counter
}

var newMessagesConsumedTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Messages consumed from topics whose handler returned no error."),
	metric.WithUnit("1"),
}

// NewMessagesConsumedTotal returns a new MessagesConsumedTotal instrument
// created on m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewMessagesConsumedTotal(m metric.Meter, opt ...metric.Int64CounterOption) (MessagesConsumedTotal, error) {
	if m == nil {
		return MessagesConsumedTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newMessagesConsumedTotalOpts
	} else {
		opt = append(opt, newMessagesConsumedTotalOpts...)
	}

	i, err := m.Int64Counter("watermill_messages_consumed_total", opt...)
	if err != nil {
		return MessagesConsumedTotal{noop.Int64Counter{}}, err
	}

	return MessagesConsumedTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (MessagesConsumedTotal) Name() string {
	return "watermill_messages_consumed_total"
}

// Unit returns the semantic convention unit of the instrument.
func (MessagesConsumedTotal) Unit() string {
	return "1"
}

// Description returns the semantic convention description of the instrument.
func (MessagesConsumedTotal) Description() string {
	return "Messages consumed from topics whose handler returned no error."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// topic: The Watermill topic the message was published to or consumed from.
//
// Optional attributes: span_id, trace_id.
func (m MessagesConsumedTotal) Add(
	ctx context.Context,
	incr int64,
	topic string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		Topic(topic),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// MessagesFailedTotal is the "watermill_messages_failed_total" instrument.
// Message operations, publish or consume, that returned an error.
type MessagesFailedTotal struct {
	metric.Int64Counter
}

var newMessagesFailedTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Message operations, publish or consume, that returned an error."),
	metric.WithUnit("1"),
}

// NewMessagesFailedTotal returns a new MessagesFailedTotal instrument created on
// m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewMessagesFailedTotal(m metric.Meter, opt ...metric.Int64CounterOption) (MessagesFailedTotal, error) {
	if m == nil {
		return MessagesFailedTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newMessagesFailedTotalOpts
	} else {
		opt = append(opt, newMessagesFailedTotalOpts...)
	}

	i, err := m.Int64Counter("watermill_messages_failed_total", opt...)
	if err != nil {
		return MessagesFailedTotal{noop.Int64Counter{}}, err
	}

	return MessagesFailedTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (MessagesFailedTotal) Name() string {
	return "watermill_messages_failed_total"
}

// Unit returns the semantic convention unit of the instrument.
func (MessagesFailedTotal) Unit() string {
	return "1"
}

// Description returns the semantic convention description of the instrument.
func (MessagesFailedTotal) Description() string {
	return "Message operations, publish or consume, that returned an error."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// stage: Which side of the message flow the failed operation belongs to.
// topic: The Watermill topic the message was published to or consumed from.
//
// Optional attributes: error, span_id, trace_id.
func (m MessagesFailedTotal) Add(
	ctx context.Context,
	incr int64,
	stage StageAttr,
	topic string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+2)
	all = append(all,
		Stage(stage),
		Topic(topic),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// MessagesPublishedTotal is the "watermill_messages_published_total" instrument.
// Messages published to topics.
type MessagesPublishedTotal struct {
	metric.Int64Counter
}

var newMessagesPublishedTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Messages published to topics."),
	metric.WithUnit("1"),
}

// NewMessagesPublishedTotal returns a new MessagesPublishedTotal instrument
// created on m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewMessagesPublishedTotal(m metric.Meter, opt ...metric.Int64CounterOption) (MessagesPublishedTotal, error) {
	if m == nil {
		return MessagesPublishedTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newMessagesPublishedTotalOpts
	} else {
		opt = append(opt, newMessagesPublishedTotalOpts...)
	}

	i, err := m.Int64Counter("watermill_messages_published_total", opt...)
	if err != nil {
		return MessagesPublishedTotal{noop.Int64Counter{}}, err
	}

	return MessagesPublishedTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (MessagesPublishedTotal) Name() string {
	return "watermill_messages_published_total"
}

// Unit returns the semantic convention unit of the instrument.
func (MessagesPublishedTotal) Unit() string {
	return "1"
}

// Description returns the semantic convention description of the instrument.
func (MessagesPublishedTotal) Description() string {
	return "Messages published to topics."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// topic: The Watermill topic the message was published to or consumed from.
//
// Optional attributes: span_id, trace_id.
func (m MessagesPublishedTotal) Add(
	ctx context.Context,
	incr int64,
	topic string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		Topic(topic),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// PublishLatencySeconds is the "watermill_publish_latency_seconds" instrument.
// Duration of one publish call, including the wrapped publisher.
type PublishLatencySeconds struct {
	metric.Float64Histogram
}

var newPublishLatencySecondsOpts = []metric.Float64HistogramOption{
	metric.WithDescription("Duration of one publish call, including the wrapped publisher."),
	metric.WithUnit("s"),
}

// NewPublishLatencySeconds returns a new PublishLatencySeconds instrument
// created on m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewPublishLatencySeconds(m metric.Meter, opt ...metric.Float64HistogramOption) (PublishLatencySeconds, error) {
	if m == nil {
		return PublishLatencySeconds{noop.Float64Histogram{}}, nil
	}

	if len(opt) == 0 {
		opt = newPublishLatencySecondsOpts
	} else {
		opt = append(opt, newPublishLatencySecondsOpts...)
	}

	i, err := m.Float64Histogram("watermill_publish_latency_seconds", opt...)
	if err != nil {
		return PublishLatencySeconds{noop.Float64Histogram{}}, err
	}

	return PublishLatencySeconds{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (PublishLatencySeconds) Name() string {
	return "watermill_publish_latency_seconds"
}

// Unit returns the semantic convention unit of the instrument.
func (PublishLatencySeconds) Unit() string {
	return "s"
}

// Description returns the semantic convention description of the instrument.
func (PublishLatencySeconds) Description() string {
	return "Duration of one publish call, including the wrapped publisher."
}

// Record records val to the current distribution for the given attributes.
// Required attributes are positional; attrs carries the optional ones.
//
// topic: The Watermill topic the message was published to or consumed from.
//
// Optional attributes: span_id, trace_id.
func (m PublishLatencySeconds) Record(
	ctx context.Context,
	val float64,
	topic string,
	attrs ...attribute.KeyValue,
) {
	if !m.Float64Histogram.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		Topic(topic),
	)
	all = append(all, attrs...)

	m.Float64Histogram.Record(ctx, val, metric.WithAttributes(all...))
}
