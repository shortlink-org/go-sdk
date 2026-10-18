// Code generated from the go-sdk semantic convention registry. DO NOT EDIT.
//
// Source: semconv/registry/postgres.yaml. Regenerate with `make -C semconv generate`.

// Package semconv holds the attribute keys and metric instruments of the
// postgres module as declared in the go-sdk semantic convention registry.
package semconv

import "go.opentelemetry.io/otel/attribute"

const (
	// ClassKey is the "class" attribute key. How the SQL classifier read the
	// statement.
	//
	// Type: Enum
	// Stability: Development
	ClassKey = attribute.Key("class")

	// ErrorTypeKey is the "error.type" attribute key. Describes a class of error
	// the operation ended with.
	//
	// Type: Enum
	// Stability: Stable
	// Note: The Go type name of the error, never its message.
	ErrorTypeKey = attribute.Key("error.type")

	// MethodKey is the "method" attribute key. How the primary's WAL position was
	// captured.
	//
	// Type: Enum
	// Stability: Development
	MethodKey = attribute.Key("method")

	// OutcomeKey is the "outcome" attribute key. How the operation ended. The set
	// of values is listed on each metric that records it.
	//
	// Type: string
	// Stability: Development
	OutcomeKey = attribute.Key("outcome")

	// ReasonKey is the "reason" attribute key. Why the routing decision came out
	// the way it did.
	//
	// Type: Enum
	// Stability: Development
	// Note: The set is closed on purpose; an open-ended reason is how a metrics
	// backend falls over.
	ReasonKey = attribute.Key("reason")

	// ServerAddressKey is the "server.address" attribute key. The replica host.
	//
	// Type: string
	// Stability: Stable
	// Note: When observed from the client side, and when communicating through an
	// intermediary, `server.address` SHOULD represent the server address behind any
	// intermediaries, for example proxies, if it's available.
	ServerAddressKey = attribute.Key("server.address")

	// StrategyKey is the "strategy" attribute key. The routing strategy the caller
	// asked for.
	//
	// Type: Enum
	// Stability: Development
	StrategyKey = attribute.Key("strategy")

	// TargetKey is the "target" attribute key. Which pool the statement was routed
	// to.
	//
	// Type: Enum
	// Stability: Development
	TargetKey = attribute.Key("target")
)

// Outcome returns a "outcome" attribute. How the operation ended. The set of
// values is listed on each metric that records it.
func Outcome(val string) attribute.KeyValue {
	return OutcomeKey.String(val)
}

// ServerAddress returns a "server.address" attribute. The replica host.
func ServerAddress(val string) attribute.KeyValue {
	return ServerAddressKey.String(val)
}

// ClassAttr is a value of the "class" attribute.
type ClassAttr string

const (
	// The statement may run on a standby.
	// Stability: Development
	ClassRead ClassAttr = "read"
	// The statement must run on the primary.
	// Stability: Development
	ClassWrite ClassAttr = "write"
	// The statement could not be understood and is treated as a write.
	// Stability: Development
	ClassUnknown ClassAttr = "unknown"
)

// Class returns a "class" attribute.
func Class(val ClassAttr) attribute.KeyValue {
	return ClassKey.String(string(val))
}

// ErrorTypeAttr is a value of the "error.type" attribute.
type ErrorTypeAttr string

const (
	// A fallback error value to be used when the instrumentation doesn't define a
	// custom value.
	// .
	// Stability: Stable
	ErrorTypeOther ErrorTypeAttr = "_OTHER"
)

// ErrorType returns a "error.type" attribute.
func ErrorType(val ErrorTypeAttr) attribute.KeyValue {
	return ErrorTypeKey.String(string(val))
}

// MethodAttr is a value of the "method" attribute.
type MethodAttr string

const (
	// The WAL position was read with its own round trip.
	// Stability: Development
	MethodStandalone MethodAttr = "standalone"
	// The WAL position was read in the same batch as the commit.
	// Stability: Development
	MethodBatchedCommit MethodAttr = "batched_commit"
)

// Method returns a "method" attribute.
func Method(val MethodAttr) attribute.KeyValue {
	return MethodKey.String(string(val))
}

// ReasonAttr is a value of the "reason" attribute.
type ReasonAttr string

const (
	// The statement writes, so it must run on the primary.
	// Stability: Development
	ReasonWrite ReasonAttr = "write"
	// The statement could not be classified and was treated as a write.
	// Stability: Development
	ReasonUnknown ReasonAttr = "unknown"
	// The context already wrote in this request, so reads stay on the primary.
	// Stability: Development
	ReasonTainted ReasonAttr = "tainted"
	// No replica had replayed the required position within the wait.
	// Stability: Development
	ReasonBehind ReasonAttr = "behind"
	// A replica had replayed the required position.
	// Stability: Development
	ReasonCaughtUp ReasonAttr = "caught_up"
	// A replica was within the configured lag budget.
	// Stability: Development
	ReasonWithinLag ReasonAttr = "within_lag"
	// The context carried no watermark tracker.
	// Stability: Development
	ReasonNoTracker ReasonAttr = "no_tracker"
	// The caller pinned the statement with a strategy.
	// Stability: Development
	ReasonExplicit ReasonAttr = "explicit"
	// No replica was eligible for reads.
	// Stability: Development
	ReasonNoHealthyReplica ReasonAttr = "no_healthy_replica"
	// The statement runs inside a transaction, which is pinned to one pool.
	// Stability: Development
	ReasonInTransaction ReasonAttr = "in_transaction"
	// Replica routing is switched off.
	// Stability: Development
	ReasonRouterDisabled ReasonAttr = "router_disabled"
	// The read may be retried on the primary after a replica failure.
	// Stability: Development
	ReasonSafeToRetry ReasonAttr = "safe_to_retry"
)

// Reason returns a "reason" attribute.
func Reason(val ReasonAttr) attribute.KeyValue {
	return ReasonKey.String(string(val))
}

// StrategyAttr is a value of the "strategy" attribute.
type StrategyAttr string

const (
	// The caller expressed no preference.
	// Stability: Development
	StrategyUnset StrategyAttr = "unset"
	// Use a replica only once it has replayed the caller's writes.
	// Stability: Development
	StrategyReadAfterWrite StrategyAttr = "read_after_write"
	// Any healthy replica within the configured lag budget.
	// Stability: Development
	StrategyStaleRead StrategyAttr = "stale_read"
	// Every statement pinned to the primary.
	// Stability: Development
	StrategyPrimary StrategyAttr = "primary"
	// A replica is required; fail rather than fall back.
	// Stability: Development
	StrategyReplica StrategyAttr = "replica"
)

// Strategy returns a "strategy" attribute.
func Strategy(val StrategyAttr) attribute.KeyValue {
	return StrategyKey.String(string(val))
}

// TargetAttr is a value of the "target" attribute.
type TargetAttr string

const (
	// The statement ran on the primary.
	// Stability: Development
	TargetPrimary TargetAttr = "primary"
	// The statement ran on a read replica.
	// Stability: Development
	TargetReplica TargetAttr = "replica"
)

// Target returns a "target" attribute.
func Target(val TargetAttr) attribute.KeyValue {
	return TargetKey.String(string(val))
}
