// Package metrics holds the OpenTelemetry instruments for read-replica
// routing.
//
// It deliberately knows nothing about routing: every call takes strings and
// numbers rather than the router's own types. That is what keeps the
// dependency one-way — the router reaches for metrics, never the reverse.
//
// The instruments and the attribute vocabulary are declared in
// semconv/registry/postgres.yaml; the semconv package next to this one is
// generated from it. This package is the routing-facing facade over it.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/semconv"
)

// meterName is the instrumentation scope. Unlike the bare "watermill" scope in
// the watermill package, new instruments use the qualified import path, which
// is the OpenTelemetry convention.
const meterName = "github.com/shortlink-org/go-sdk/db/drivers/postgres"

// instrumentCount is how many instruments New builds.
const instrumentCount = 7

// Two values are deliberately absent from every attribute set. A WAL position
// rises forever, so using one as an attribute is an unbounded-cardinality
// incident waiting to happen — and it does not fit a float64 gauge on a
// long-lived cluster anyway. The actor key is personal data. Both belong on a
// span, where they cost nothing and are what you actually want while
// debugging.

// Routing targets, as metric attribute values.
const (
	TargetPrimary = string(semconv.TargetPrimary)
	TargetReplica = string(semconv.TargetReplica)
)

// Reasons a routing decision came out the way it did. The set is closed on
// purpose: an open-ended reason string is how a metrics backend falls over.
const (
	ReasonWrite            = string(semconv.ReasonWrite)
	ReasonUnknown          = string(semconv.ReasonUnknown)
	ReasonTainted          = string(semconv.ReasonTainted)
	ReasonBehind           = string(semconv.ReasonBehind)
	ReasonCaughtUp         = string(semconv.ReasonCaughtUp)
	ReasonWithinLag        = string(semconv.ReasonWithinLag)
	ReasonNoTracker        = string(semconv.ReasonNoTracker)
	ReasonExplicit         = string(semconv.ReasonExplicit)
	ReasonNoHealthyReplica = string(semconv.ReasonNoHealthyReplica)
	ReasonInTransaction    = string(semconv.ReasonInTransaction)
	ReasonRouterDisabled   = string(semconv.ReasonRouterDisabled)
	ReasonSafeToRetry      = string(semconv.ReasonSafeToRetry)
)

// Capture methods, for the watermark histogram.
const (
	CaptureStandalone = string(semconv.MethodStandalone)
	CaptureBatched    = string(semconv.MethodBatchedCommit)
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
	decisions      semconv.RouteDecisionsTotal
	fallbacks      semconv.RouteFallbacksTotal
	probeFailures  semconv.ReplicaProbeFailuresTotal
	promotions     semconv.ReplicaPromotionsTotal
	probeDuration  semconv.ReplicaProbeDurationSeconds
	captureLatency semconv.WatermarkCaptureDurationSeconds
	gateWait       semconv.ConsistencyGateWaitSeconds

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

	built.decisions, err = semconv.NewRouteDecisionsTotal(meter)
	errs = append(errs, err)

	built.fallbacks, err = semconv.NewRouteFallbacksTotal(meter)
	errs = append(errs, err)

	built.probeFailures, err = semconv.NewReplicaProbeFailuresTotal(meter)
	errs = append(errs, err)

	built.promotions, err = semconv.NewReplicaPromotionsTotal(meter)
	errs = append(errs, err)

	built.probeDuration, err = semconv.NewReplicaProbeDurationSeconds(meter)
	errs = append(errs, err)

	built.captureLatency, err = semconv.NewWatermarkCaptureDurationSeconds(meter)
	errs = append(errs, err)

	built.gateWait, err = semconv.NewConsistencyGateWaitSeconds(meter)
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

	lag, err := semconv.NewReplicaLagBytes(meter)
	if err != nil {
		return fmt.Errorf("%w: replica lag gauge: %w", ErrInstruments, err)
	}

	eligible, err := semconv.NewReplicaUp(meter)
	if err != nil {
		return fmt.Errorf("%w: replica up gauge: %w", ErrInstruments, err)
	}

	m.registration, err = meter.RegisterCallback(
		func(_ context.Context, observer metric.Observer) error {
			for _, health := range snapshot() {
				attrs := metric.WithAttributes(semconv.ServerAddress(health.Host))

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
	m.decisions.Add(ctx, 1,
		semconv.ClassAttr(decision.Class),
		semconv.ReasonAttr(decision.Reason),
		semconv.StrategyAttr(decision.Strategy),
		semconv.TargetAttr(decision.Target),
	)

	if decision.Fallback {
		m.Fallback(ctx, decision.Reason)
	}
}

// Fallback counts a read that wanted a replica and got the primary.
func (m *Metrics) Fallback(ctx context.Context, reason string) {
	m.fallbacks.Add(ctx, 1, semconv.ReasonAttr(reason))
}

// ProbeFailed counts a failed replica health probe.
func (m *Metrics) ProbeFailed(ctx context.Context, host string, err error) {
	m.probeFailures.Add(ctx, 1, semconv.ErrorTypeAttr(errorType(err)), host)
}

// ProbeDuration records how long one health probe took.
func (m *Metrics) ProbeDuration(ctx context.Context, host string, elapsed time.Duration) {
	m.probeDuration.Record(ctx, elapsed.Seconds(), host)
}

// Promotion counts a replica observed leaving recovery.
func (m *Metrics) Promotion(ctx context.Context, host string) {
	m.promotions.Add(ctx, 1, host)
}

// CaptureDuration records how long it took to read the primary's WAL position.
func (m *Metrics) CaptureDuration(ctx context.Context, method string, elapsed time.Duration, err error) {
	m.captureLatency.Record(ctx, elapsed.Seconds(), semconv.MethodAttr(method), outcome(err))
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
