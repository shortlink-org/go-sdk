// Code generated from the go-sdk semantic convention registry. DO NOT EDIT.
//
// Source: semconv/registry/postgres.yaml. Regenerate with `make -C semconv generate`.

package semconv

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// ConsistencyGateWaitSeconds is the "postgres_consistency_gate_wait_seconds"
// instrument. Time spent waiting for a replica to replay a required WAL
// position.
type ConsistencyGateWaitSeconds struct {
	metric.Float64Histogram
}

var newConsistencyGateWaitSecondsOpts = []metric.Float64HistogramOption{
	metric.WithDescription("Time spent waiting for a replica to replay a required WAL position."),
	metric.WithUnit("s"),
}

// NewConsistencyGateWaitSeconds returns a new ConsistencyGateWaitSeconds
// instrument created on m. A nil m yields a no-op instrument, which is what
// callers without observability get.
func NewConsistencyGateWaitSeconds(m metric.Meter, opt ...metric.Float64HistogramOption) (ConsistencyGateWaitSeconds, error) {
	if m == nil {
		return ConsistencyGateWaitSeconds{noop.Float64Histogram{}}, nil
	}

	if len(opt) == 0 {
		opt = newConsistencyGateWaitSecondsOpts
	} else {
		opt = append(opt, newConsistencyGateWaitSecondsOpts...)
	}

	i, err := m.Float64Histogram("postgres_consistency_gate_wait_seconds", opt...)
	if err != nil {
		return ConsistencyGateWaitSeconds{noop.Float64Histogram{}}, err
	}

	return ConsistencyGateWaitSeconds{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ConsistencyGateWaitSeconds) Name() string {
	return "postgres_consistency_gate_wait_seconds"
}

// Unit returns the semantic convention unit of the instrument.
func (ConsistencyGateWaitSeconds) Unit() string {
	return "s"
}

// Description returns the semantic convention description of the instrument.
func (ConsistencyGateWaitSeconds) Description() string {
	return "Time spent waiting for a replica to replay a required WAL position."
}

// Record records val to the current distribution for the given attributes.
// Required attributes are positional; attrs carries the optional ones.
func (m ConsistencyGateWaitSeconds) Record(
	ctx context.Context,
	val float64,
	attrs ...attribute.KeyValue,
) {
	if !m.Float64Histogram.Enabled(ctx) {
		return
	}

	m.Float64Histogram.Record(ctx, val, metric.WithAttributes(attrs...))
}

// ReplicaLagBytes is the "postgres_replica_lag_bytes" instrument. WAL bytes by
// which a replica trails the primary.
//
// It is observed from a callback: register it with metric.Meter.RegisterCallback
// and report values through the metric.Observer.
type ReplicaLagBytes struct {
	metric.Int64ObservableGauge
}

var newReplicaLagBytesOpts = []metric.Int64ObservableGaugeOption{
	metric.WithDescription("WAL bytes by which a replica trails the primary."),
	metric.WithUnit("By"),
}

// NewReplicaLagBytes returns a new ReplicaLagBytes instrument created on m. A
// nil m yields a no-op instrument, which is what callers without observability
// get.
func NewReplicaLagBytes(m metric.Meter, opt ...metric.Int64ObservableGaugeOption) (ReplicaLagBytes, error) {
	if m == nil {
		return ReplicaLagBytes{noop.Int64ObservableGauge{}}, nil
	}

	if len(opt) == 0 {
		opt = newReplicaLagBytesOpts
	} else {
		opt = append(opt, newReplicaLagBytesOpts...)
	}

	i, err := m.Int64ObservableGauge("postgres_replica_lag_bytes", opt...)
	if err != nil {
		return ReplicaLagBytes{noop.Int64ObservableGauge{}}, err
	}

	return ReplicaLagBytes{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ReplicaLagBytes) Name() string {
	return "postgres_replica_lag_bytes"
}

// Unit returns the semantic convention unit of the instrument.
func (ReplicaLagBytes) Unit() string {
	return "By"
}

// Description returns the semantic convention description of the instrument.
func (ReplicaLagBytes) Description() string {
	return "WAL bytes by which a replica trails the primary."
}

// ReplicaProbeDurationSeconds is the "postgres_replica_probe_duration_seconds"
// instrument. Duration of one replica health probe.
type ReplicaProbeDurationSeconds struct {
	metric.Float64Histogram
}

var newReplicaProbeDurationSecondsOpts = []metric.Float64HistogramOption{
	metric.WithDescription("Duration of one replica health probe."),
	metric.WithUnit("s"),
}

// NewReplicaProbeDurationSeconds returns a new ReplicaProbeDurationSeconds
// instrument created on m. A nil m yields a no-op instrument, which is what
// callers without observability get.
func NewReplicaProbeDurationSeconds(m metric.Meter, opt ...metric.Float64HistogramOption) (ReplicaProbeDurationSeconds, error) {
	if m == nil {
		return ReplicaProbeDurationSeconds{noop.Float64Histogram{}}, nil
	}

	if len(opt) == 0 {
		opt = newReplicaProbeDurationSecondsOpts
	} else {
		opt = append(opt, newReplicaProbeDurationSecondsOpts...)
	}

	i, err := m.Float64Histogram("postgres_replica_probe_duration_seconds", opt...)
	if err != nil {
		return ReplicaProbeDurationSeconds{noop.Float64Histogram{}}, err
	}

	return ReplicaProbeDurationSeconds{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ReplicaProbeDurationSeconds) Name() string {
	return "postgres_replica_probe_duration_seconds"
}

// Unit returns the semantic convention unit of the instrument.
func (ReplicaProbeDurationSeconds) Unit() string {
	return "s"
}

// Description returns the semantic convention description of the instrument.
func (ReplicaProbeDurationSeconds) Description() string {
	return "Duration of one replica health probe."
}

// Record records val to the current distribution for the given attributes.
// Required attributes are positional; attrs carries the optional ones.
//
// serverAddress: The replica host the probe went to.
func (m ReplicaProbeDurationSeconds) Record(
	ctx context.Context,
	val float64,
	serverAddress string,
	attrs ...attribute.KeyValue,
) {
	if !m.Float64Histogram.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		ServerAddress(serverAddress),
	)
	all = append(all, attrs...)

	m.Float64Histogram.Record(ctx, val, metric.WithAttributes(all...))
}

// ReplicaProbeFailuresTotal is the "postgres_replica_probe_failures_total"
// instrument. Replica health probes that failed.
type ReplicaProbeFailuresTotal struct {
	metric.Int64Counter
}

var newReplicaProbeFailuresTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Replica health probes that failed."),
	metric.WithUnit("1"),
}

// NewReplicaProbeFailuresTotal returns a new ReplicaProbeFailuresTotal
// instrument created on m. A nil m yields a no-op instrument, which is what
// callers without observability get.
func NewReplicaProbeFailuresTotal(m metric.Meter, opt ...metric.Int64CounterOption) (ReplicaProbeFailuresTotal, error) {
	if m == nil {
		return ReplicaProbeFailuresTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newReplicaProbeFailuresTotalOpts
	} else {
		opt = append(opt, newReplicaProbeFailuresTotalOpts...)
	}

	i, err := m.Int64Counter("postgres_replica_probe_failures_total", opt...)
	if err != nil {
		return ReplicaProbeFailuresTotal{noop.Int64Counter{}}, err
	}

	return ReplicaProbeFailuresTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ReplicaProbeFailuresTotal) Name() string {
	return "postgres_replica_probe_failures_total"
}

// Unit returns the semantic convention unit of the instrument.
func (ReplicaProbeFailuresTotal) Unit() string {
	return "1"
}

// Description returns the semantic convention description of the instrument.
func (ReplicaProbeFailuresTotal) Description() string {
	return "Replica health probes that failed."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// errorType: Describes a class of error the operation ended with.
// serverAddress: The replica host the probe went to.
func (m ReplicaProbeFailuresTotal) Add(
	ctx context.Context,
	incr int64,
	errorType ErrorTypeAttr,
	serverAddress string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+2)
	all = append(all,
		ErrorType(errorType),
		ServerAddress(serverAddress),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// ReplicaPromotionsTotal is the "postgres_replica_promotions_total" instrument.
// Replicas observed leaving recovery, i.e. promoted and quarantined.
type ReplicaPromotionsTotal struct {
	metric.Int64Counter
}

var newReplicaPromotionsTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Replicas observed leaving recovery, i.e. promoted and quarantined."),
	metric.WithUnit("1"),
}

// NewReplicaPromotionsTotal returns a new ReplicaPromotionsTotal instrument
// created on m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewReplicaPromotionsTotal(m metric.Meter, opt ...metric.Int64CounterOption) (ReplicaPromotionsTotal, error) {
	if m == nil {
		return ReplicaPromotionsTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newReplicaPromotionsTotalOpts
	} else {
		opt = append(opt, newReplicaPromotionsTotalOpts...)
	}

	i, err := m.Int64Counter("postgres_replica_promotions_total", opt...)
	if err != nil {
		return ReplicaPromotionsTotal{noop.Int64Counter{}}, err
	}

	return ReplicaPromotionsTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ReplicaPromotionsTotal) Name() string {
	return "postgres_replica_promotions_total"
}

// Unit returns the semantic convention unit of the instrument.
func (ReplicaPromotionsTotal) Unit() string {
	return "1"
}

// Description returns the semantic convention description of the instrument.
func (ReplicaPromotionsTotal) Description() string {
	return "Replicas observed leaving recovery, i.e. promoted and quarantined."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// serverAddress: The replica host that was promoted.
func (m ReplicaPromotionsTotal) Add(
	ctx context.Context,
	incr int64,
	serverAddress string,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		ServerAddress(serverAddress),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// ReplicaUp is the "postgres_replica_up" instrument. Whether a replica is
// currently eligible for reads (0 or 1).
//
// No unit on purpose. The OpenTelemetry-to-Prometheus naming rules append
// `_ratio` to a gauge whose unit is `1`, which would export this state flag as
// `postgres_replica_up_ratio`, a name nobody would go looking for.
//
// It is observed from a callback: register it with metric.Meter.RegisterCallback
// and report values through the metric.Observer.
type ReplicaUp struct {
	metric.Int64ObservableGauge
}

var newReplicaUpOpts = []metric.Int64ObservableGaugeOption{
	metric.WithDescription("Whether a replica is currently eligible for reads (0 or 1)."),
}

// NewReplicaUp returns a new ReplicaUp instrument created on m. A nil m yields a
// no-op instrument, which is what callers without observability get.
func NewReplicaUp(m metric.Meter, opt ...metric.Int64ObservableGaugeOption) (ReplicaUp, error) {
	if m == nil {
		return ReplicaUp{noop.Int64ObservableGauge{}}, nil
	}

	if len(opt) == 0 {
		opt = newReplicaUpOpts
	} else {
		opt = append(opt, newReplicaUpOpts...)
	}

	i, err := m.Int64ObservableGauge("postgres_replica_up", opt...)
	if err != nil {
		return ReplicaUp{noop.Int64ObservableGauge{}}, err
	}

	return ReplicaUp{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ReplicaUp) Name() string {
	return "postgres_replica_up"
}

// Unit returns the semantic convention unit of the instrument.
func (ReplicaUp) Unit() string {
	return ""
}

// Description returns the semantic convention description of the instrument.
func (ReplicaUp) Description() string {
	return "Whether a replica is currently eligible for reads (0 or 1)."
}

// RouteDecisionsTotal is the "postgres_route_decisions_total" instrument.
// Statements routed, by target and reason.
type RouteDecisionsTotal struct {
	metric.Int64Counter
}

var newRouteDecisionsTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Statements routed, by target and reason."),
	metric.WithUnit("1"),
}

// NewRouteDecisionsTotal returns a new RouteDecisionsTotal instrument created on
// m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewRouteDecisionsTotal(m metric.Meter, opt ...metric.Int64CounterOption) (RouteDecisionsTotal, error) {
	if m == nil {
		return RouteDecisionsTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newRouteDecisionsTotalOpts
	} else {
		opt = append(opt, newRouteDecisionsTotalOpts...)
	}

	i, err := m.Int64Counter("postgres_route_decisions_total", opt...)
	if err != nil {
		return RouteDecisionsTotal{noop.Int64Counter{}}, err
	}

	return RouteDecisionsTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (RouteDecisionsTotal) Name() string {
	return "postgres_route_decisions_total"
}

// Unit returns the semantic convention unit of the instrument.
func (RouteDecisionsTotal) Unit() string {
	return "1"
}

// Description returns the semantic convention description of the instrument.
func (RouteDecisionsTotal) Description() string {
	return "Statements routed, by target and reason."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// class: How the SQL classifier read the statement.
// reason: Why the routing decision came out the way it did.
// strategy: The routing strategy the caller asked for.
// target: Which pool the statement was routed to.
func (m RouteDecisionsTotal) Add(
	ctx context.Context,
	incr int64,
	class ClassAttr,
	reason ReasonAttr,
	strategy StrategyAttr,
	target TargetAttr,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+4)
	all = append(all,
		Class(class),
		Reason(reason),
		Strategy(strategy),
		Target(target),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// RouteFallbacksTotal is the "postgres_route_fallbacks_total" instrument. Reads
// that wanted a replica and got the primary.
//
// This is the signal that tells you whether replica routing is paying for
// itself.
type RouteFallbacksTotal struct {
	metric.Int64Counter
}

var newRouteFallbacksTotalOpts = []metric.Int64CounterOption{
	metric.WithDescription("Reads that wanted a replica and got the primary."),
	metric.WithUnit("1"),
}

// NewRouteFallbacksTotal returns a new RouteFallbacksTotal instrument created on
// m. A nil m yields a no-op instrument, which is what callers without
// observability get.
func NewRouteFallbacksTotal(m metric.Meter, opt ...metric.Int64CounterOption) (RouteFallbacksTotal, error) {
	if m == nil {
		return RouteFallbacksTotal{noop.Int64Counter{}}, nil
	}

	if len(opt) == 0 {
		opt = newRouteFallbacksTotalOpts
	} else {
		opt = append(opt, newRouteFallbacksTotalOpts...)
	}

	i, err := m.Int64Counter("postgres_route_fallbacks_total", opt...)
	if err != nil {
		return RouteFallbacksTotal{noop.Int64Counter{}}, err
	}

	return RouteFallbacksTotal{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (RouteFallbacksTotal) Name() string {
	return "postgres_route_fallbacks_total"
}

// Unit returns the semantic convention unit of the instrument.
func (RouteFallbacksTotal) Unit() string {
	return "1"
}

// Description returns the semantic convention description of the instrument.
func (RouteFallbacksTotal) Description() string {
	return "Reads that wanted a replica and got the primary."
}

// Add adds incr to the existing count for the given attributes. Required
// attributes are positional; attrs carries the optional ones.
//
// reason: Why the routing decision came out the way it did.
func (m RouteFallbacksTotal) Add(
	ctx context.Context,
	incr int64,
	reason ReasonAttr,
	attrs ...attribute.KeyValue,
) {
	if !m.Int64Counter.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all,
		Reason(reason),
	)
	all = append(all, attrs...)

	m.Int64Counter.Add(ctx, incr, metric.WithAttributes(all...))
}

// WatermarkCaptureDurationSeconds is the
// "postgres_watermark_capture_duration_seconds" instrument. Duration of
// capturing the primary's WAL position.
//
// `outcome` is `ok` or `error`.
type WatermarkCaptureDurationSeconds struct {
	metric.Float64Histogram
}

var newWatermarkCaptureDurationSecondsOpts = []metric.Float64HistogramOption{
	metric.WithDescription("Duration of capturing the primary's WAL position."),
	metric.WithUnit("s"),
}

// NewWatermarkCaptureDurationSeconds returns a new
// WatermarkCaptureDurationSeconds instrument created on m. A nil m yields a
// no-op instrument, which is what callers without observability get.
func NewWatermarkCaptureDurationSeconds(m metric.Meter, opt ...metric.Float64HistogramOption) (WatermarkCaptureDurationSeconds, error) {
	if m == nil {
		return WatermarkCaptureDurationSeconds{noop.Float64Histogram{}}, nil
	}

	if len(opt) == 0 {
		opt = newWatermarkCaptureDurationSecondsOpts
	} else {
		opt = append(opt, newWatermarkCaptureDurationSecondsOpts...)
	}

	i, err := m.Float64Histogram("postgres_watermark_capture_duration_seconds", opt...)
	if err != nil {
		return WatermarkCaptureDurationSeconds{noop.Float64Histogram{}}, err
	}

	return WatermarkCaptureDurationSeconds{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (WatermarkCaptureDurationSeconds) Name() string {
	return "postgres_watermark_capture_duration_seconds"
}

// Unit returns the semantic convention unit of the instrument.
func (WatermarkCaptureDurationSeconds) Unit() string {
	return "s"
}

// Description returns the semantic convention description of the instrument.
func (WatermarkCaptureDurationSeconds) Description() string {
	return "Duration of capturing the primary's WAL position."
}

// Record records val to the current distribution for the given attributes.
// Required attributes are positional; attrs carries the optional ones.
//
// method: How the primary's WAL position was captured.
// outcome: How the operation ended. The set of values is listed on each metric
// that records it.
func (m WatermarkCaptureDurationSeconds) Record(
	ctx context.Context,
	val float64,
	method MethodAttr,
	outcome string,
	attrs ...attribute.KeyValue,
) {
	if !m.Float64Histogram.Enabled(ctx) {
		return
	}

	all := make([]attribute.KeyValue, 0, len(attrs)+2)
	all = append(all,
		Method(method),
		Outcome(outcome),
	)
	all = append(all, attrs...)

	m.Float64Histogram.Record(ctx, val, metric.WithAttributes(all...))
}
