// Package metrics holds the OpenTelemetry instruments for read-replica
// routing.
//
// It deliberately knows nothing about routing: every call takes strings and
// numbers rather than the router's own types. That is what keeps the
// dependency one-way — the router reaches for metrics, never the reverse — and
// it also means the attribute vocabulary is defined in one place, next to the
// instruments it constrains.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// meterName is the instrumentation scope. Unlike the bare "watermill" scope in
// the watermill package, new instruments use the qualified import path, which
// is the OpenTelemetry convention.
const meterName = "github.com/shortlink-org/go-sdk/db/drivers/postgres"

// instrumentCount is how many instruments New builds.
const instrumentCount = 7

// Attribute keys, of which only the server address follows the OpenTelemetry
// semantic conventions; the rest are specific to routing.
//
// Two values are deliberately absent from every attribute set. A WAL position
// rises forever, so using one as an attribute is an unbounded-cardinality
// incident waiting to happen — and it does not fit a float64 gauge on a
// long-lived cluster anyway. The actor key is personal data. Both belong on a
// span, where they cost nothing and are what you actually want while
// debugging.
const (
	attrTarget      = attribute.Key("target")
	attrReason      = attribute.Key("reason")
	attrClass       = attribute.Key("class")
	attrStrategy    = attribute.Key("strategy")
	attrServer      = attribute.Key("server.address")
	attrErrorType   = attribute.Key("error.type")
	attrOutcome     = attribute.Key("outcome")
	attrCaptureKind = attribute.Key("method")
)

// Routing targets, as metric attribute values.
const (
	TargetPrimary = "primary"
	TargetReplica = "replica"
)

// Reasons a routing decision came out the way it did. The set is closed on
// purpose: an open-ended reason string is how a metrics backend falls over.
const (
	ReasonWrite            = "write"
	ReasonUnknown          = "unknown"
	ReasonTainted          = "tainted"
	ReasonBehind           = "behind"
	ReasonCaughtUp         = "caught_up"
	ReasonWithinLag        = "within_lag"
	ReasonNoTracker        = "no_tracker"
	ReasonExplicit         = "explicit"
	ReasonNoHealthyReplica = "no_healthy_replica"
	ReasonInTransaction    = "in_transaction"
	ReasonRouterDisabled   = "router_disabled"
	ReasonSafeToRetry      = "safe_to_retry"
)

// Capture methods, for the watermark histogram.
const (
	CaptureStandalone = "standalone"
	CaptureBatched    = "batched_commit"
)

// ErrInstruments reports a failure to build the instruments.
var ErrInstruments = errors.New("failed to create replica routing instruments")

// Decision is one routing decision, reduced to the attributes it is counted by.
type Decision struct {
	Target   string
	Reason   string
	Class    string
	Strategy string

	// Fallback marks a read that wanted a replica and got the primary. It is
	// counted separately because it is the signal that tells you whether the
	// feature is paying for itself.
	Fallback bool
}

// Health is one replica, as the gauges observe it.
type Health struct {
	Host     string
	LagBytes int64
	Up       bool
}

// Metrics holds the instruments. Every field is non-nil after New returns, so
// call sites never branch on nil.
type Metrics struct {
	decisions      metric.Int64Counter
	fallbacks      metric.Int64Counter
	probeFailures  metric.Int64Counter
	promotions     metric.Int64Counter
	probeDuration  metric.Float64Histogram
	captureLatency metric.Float64Histogram
	gateWait       metric.Float64Histogram

	// registration is the observable-gauge callback. It must be unregistered
	// on Close or the meter provider keeps the snapshot closure alive.
	registration metric.Registration
}

// New builds the instruments. A nil provider yields no-op instruments, which
// is what tests and callers without observability get.
func New(provider *sdkmetric.MeterProvider) (*Metrics, error) {
	meter := meterFor(provider)

	var (
		built Metrics
		err   error
	)

	errs := make([]error, 0, instrumentCount)

	built.decisions, err = meter.Int64Counter(
		"postgres_route_decisions_total",
		metric.WithDescription("Statements routed, by target and reason."),
		metric.WithUnit("1"),
	)
	errs = append(errs, err)

	built.fallbacks, err = meter.Int64Counter(
		"postgres_route_fallbacks_total",
		metric.WithDescription("Reads that wanted a replica and got the primary."),
		metric.WithUnit("1"),
	)
	errs = append(errs, err)

	built.probeFailures, err = meter.Int64Counter(
		"postgres_replica_probe_failures_total",
		metric.WithDescription("Replica health probes that failed."),
		metric.WithUnit("1"),
	)
	errs = append(errs, err)

	built.promotions, err = meter.Int64Counter(
		"postgres_replica_promotions_total",
		metric.WithDescription("Replicas observed leaving recovery, i.e. promoted and quarantined."),
		metric.WithUnit("1"),
	)
	errs = append(errs, err)

	built.probeDuration, err = meter.Float64Histogram(
		"postgres_replica_probe_duration_seconds",
		metric.WithDescription("Duration of one replica health probe."),
		metric.WithUnit("s"),
	)
	errs = append(errs, err)

	built.captureLatency, err = meter.Float64Histogram(
		"postgres_watermark_capture_duration_seconds",
		metric.WithDescription("Duration of capturing the primary's WAL position."),
		metric.WithUnit("s"),
	)
	errs = append(errs, err)

	built.gateWait, err = meter.Float64Histogram(
		"postgres_consistency_gate_wait_seconds",
		metric.WithDescription("Time spent waiting for a replica to replay a required WAL position."),
		metric.WithUnit("s"),
	)
	errs = append(errs, err)

	joined := errors.Join(errs...)
	if joined != nil {
		return nil, fmt.Errorf("%w: %w", ErrInstruments, joined)
	}

	return &built, nil
}

//nolint:ireturn // metric.Meter is the otel API's own type
func meterFor(provider *sdkmetric.MeterProvider) metric.Meter {
	if provider == nil {
		return noop.NewMeterProvider().Meter(meterName)
	}

	return provider.Meter(meterName)
}

// Observe registers the gauges that read the poller's cached samples.
//
// It takes a snapshot function rather than the poller itself, which is what
// lets this package stay ignorant of routing. The closure must not block: it
// runs on the collection path.
func (m *Metrics) Observe(provider *sdkmetric.MeterProvider, snapshot func() []Health) error {
	if provider == nil || snapshot == nil {
		return nil
	}

	meter := provider.Meter(meterName)

	lag, err := meter.Int64ObservableGauge(
		"postgres_replica_lag_bytes",
		metric.WithDescription("WAL bytes by which a replica trails the primary."),
		metric.WithUnit("By"),
	)
	if err != nil {
		return fmt.Errorf("%w: replica lag gauge: %w", ErrInstruments, err)
	}

	// No unit on purpose. The OpenTelemetry-to-Prometheus naming rules append
	// _ratio to a gauge whose unit is "1", which would export this state flag
	// as postgres_replica_up_ratio — a name nobody would go looking for.
	eligible, err := meter.Int64ObservableGauge(
		"postgres_replica_up",
		metric.WithDescription("Whether a replica is currently eligible for reads (0 or 1)."),
	)
	if err != nil {
		return fmt.Errorf("%w: replica up gauge: %w", ErrInstruments, err)
	}

	m.registration, err = meter.RegisterCallback(
		func(_ context.Context, observer metric.Observer) error {
			for _, health := range snapshot() {
				attrs := metric.WithAttributes(attrServer.String(health.Host))

				observer.ObserveInt64(lag, health.LagBytes, attrs)
				observer.ObserveInt64(eligible, boolToInt64(health.Up), attrs)
			}

			return nil
		},
		lag, eligible,
	)
	if err != nil {
		return fmt.Errorf("%w: replica gauges: %w", ErrInstruments, err)
	}

	return nil
}

// Close unregisters the gauge callback. Without it the meter provider holds a
// reference to the snapshot closure for the lifetime of the process, which
// shows up as a leak in tests and as a slow memory climb in a service that
// rebuilds stores.
func (m *Metrics) Close() {
	if m == nil || m.registration == nil {
		return
	}

	//nolint:errcheck // unregistering twice is the only failure, and it is harmless
	_ = m.registration.Unregister()
	m.registration = nil
}

// RecordDecision counts one routing decision.
func (m *Metrics) RecordDecision(ctx context.Context, decision Decision) {
	m.decisions.Add(ctx, 1, metric.WithAttributes(
		attrTarget.String(decision.Target),
		attrReason.String(decision.Reason),
		attrClass.String(decision.Class),
		attrStrategy.String(decision.Strategy),
	))

	if decision.Fallback {
		m.Fallback(ctx, decision.Reason)
	}
}

// Fallback counts a read that wanted a replica and got the primary.
func (m *Metrics) Fallback(ctx context.Context, reason string) {
	m.fallbacks.Add(ctx, 1, metric.WithAttributes(attrReason.String(reason)))
}

// ProbeFailed counts a failed replica health probe.
func (m *Metrics) ProbeFailed(ctx context.Context, host string, err error) {
	m.probeFailures.Add(ctx, 1, metric.WithAttributes(
		attrServer.String(host),
		attrErrorType.String(errorType(err)),
	))
}

// ProbeDuration records how long one health probe took.
func (m *Metrics) ProbeDuration(ctx context.Context, host string, elapsed time.Duration) {
	m.probeDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrServer.String(host)))
}

// Promotion counts a replica observed leaving recovery.
func (m *Metrics) Promotion(ctx context.Context, host string) {
	m.promotions.Add(ctx, 1, metric.WithAttributes(attrServer.String(host)))
}

// CaptureDuration records how long it took to read the primary's WAL position.
func (m *Metrics) CaptureDuration(ctx context.Context, method string, elapsed time.Duration, err error) {
	m.captureLatency.Record(ctx, elapsed.Seconds(), metric.WithAttributes(
		attrCaptureKind.String(method),
		attrOutcome.String(outcome(err)),
	))
}

// GateWait records time spent waiting for a replica to replay a position.
func (m *Metrics) GateWait(ctx context.Context, elapsed time.Duration) {
	m.gateWait.Record(ctx, elapsed.Seconds())
}

// errorType keeps error.type to a bounded set of Go type names rather than
// error messages, which carry values and would blow up cardinality.
func errorType(err error) string {
	if err == nil {
		return "none"
	}

	return fmt.Sprintf("%T", err)
}

func outcome(err error) string {
	if err != nil {
		return "error"
	}

	return "ok"
}

func boolToInt64(value bool) int64 {
	if value {
		return 1
	}

	return 0
}
