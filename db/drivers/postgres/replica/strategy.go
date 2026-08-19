package replica

import (
	"context"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/metrics"
)

// Strategy selects which pool a read is allowed to run on. It never affects a
// write: a write always goes to the primary, and asking for the opposite is an
// error rather than a silent redirection.
type Strategy uint8

const (
	// StrategyUnset is the zero value: the caller expressed no preference.
	// Resolution falls back to the tracker in the context, or — when there is
	// none — to the router's NoTrackerPolicy.
	StrategyUnset Strategy = iota

	// StrategyReadAfterWrite routes to a replica only once that replica has
	// replayed every write this context is known to have made. It is what you
	// want almost everywhere.
	StrategyReadAfterWrite

	// StrategyStaleRead accepts any healthy replica within the configured lag
	// threshold, regardless of what this context has written. Use it for reads
	// that are allowed to be a little behind — dashboards, search, exports.
	StrategyStaleRead

	// StrategyPrimary pins every statement to the primary.
	StrategyPrimary

	// StrategyReplica requires a replica and fails with ErrNoHealthyReplica
	// rather than falling back. Use it where silently melting the primary
	// would be worse than shedding the read.
	StrategyReplica
)

// String implements fmt.Stringer. The values are used as metric attributes, so
// they are lowercase and stable.
func (s Strategy) String() string {
	switch s {
	case StrategyReadAfterWrite:
		return "read_after_write"
	case StrategyStaleRead:
		return "stale_read"
	case StrategyPrimary:
		return metrics.TargetPrimary
	case StrategyReplica:
		return metrics.TargetReplica
	case StrategyUnset:
		return "unset"
	default:
		return "unset"
	}
}

type strategyKey struct{}

// WithStrategy returns a context whose reads use s.
func WithStrategy(ctx context.Context, s Strategy) context.Context {
	return context.WithValue(ctx, strategyKey{}, s)
}

// StrategyFromContext returns the strategy carried by ctx, or StrategyUnset.
func StrategyFromContext(ctx context.Context) Strategy {
	strategy, ok := ctx.Value(strategyKey{}).(Strategy)
	if !ok {
		return StrategyUnset
	}

	return strategy
}

// Stale returns a context whose reads accept replica lag.
//
//	rows, err := router.Query(postgres.Stale(ctx), listAllOrders)
func Stale(ctx context.Context) context.Context {
	return WithStrategy(ctx, StrategyStaleRead)
}

// OnPrimary returns a context whose reads are pinned to the primary.
func OnPrimary(ctx context.Context) context.Context {
	return WithStrategy(ctx, StrategyPrimary)
}

// OnReplica returns a context whose reads require a replica and fail loudly
// when none is healthy.
func OnReplica(ctx context.Context) context.Context {
	return WithStrategy(ctx, StrategyReplica)
}

// NoTrackerPolicy decides where a read goes when its context carries neither a
// tracker nor an explicit strategy — that is, when no boundary middleware has
// scoped the unit of work.
type NoTrackerPolicy uint8

const (
	// NoTrackerPrimary sends such reads to the primary. It is the default: an
	// unscoped read is one nobody has reasoned about, and the cost of being
	// wrong is a stale row that surfaces as an intermittent bug somewhere else
	// entirely.
	NoTrackerPrimary NoTrackerPolicy = iota

	// NoTrackerReplica sends such reads to a replica. Choose it for a service
	// that never writes, where scoping every read would be ceremony.
	NoTrackerReplica
)

// String implements fmt.Stringer.
func (p NoTrackerPolicy) String() string {
	switch p {
	case NoTrackerPrimary:
		return metrics.TargetPrimary
	case NoTrackerReplica:
		return metrics.TargetReplica
	default:
		return metrics.TargetPrimary
	}
}
