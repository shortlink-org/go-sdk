package replica

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/metrics"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// clusterLineage owns the rules for deciding whether a portable WAL token
// belongs to this PostgreSQL history. Keeping those rules next to the data
// prevents callers from accidentally comparing a bare LSN after a failover.
type clusterLineage struct {
	systemID uint64
	timeline uint32
}

func (l clusterLineage) known() bool { return l.timeline != 0 }

func (l clusterLineage) token(position wal.LSN, issuedAt time.Time) wal.Token {
	return wal.Token{SystemID: l.systemID, Timeline: l.timeline, LSN: position, IssuedAt: issuedAt}
}

func (l clusterLineage) accepts(token wal.Token) bool {
	if !l.known() || token.Timeline != l.timeline {
		return false
	}

	// A zero system identifier means pg_control_system() was unavailable.
	// Timeline comparison still detects every promotion in that case.
	return l.systemID == 0 || token.SystemID == 0 || token.SystemID == l.systemID
}

// primarySample is a position together with the moment it was observed. Its
// freshness is part of the value: a stale LSN must never be used for clamping.
type primarySample struct {
	position wal.LSN
	observed time.Time
}

func (s primarySample) fresh(staleAfter time.Duration) bool {
	return s.position != 0 && !s.observed.IsZero() && time.Since(s.observed) <= staleAfter
}

func (s primarySample) clamp(position wal.LSN) wal.LSN {
	if position > s.position {
		return s.position
	}

	return position
}

func (s primarySample) lagBehind(replay wal.LSN, staleAfter time.Duration) int64 {
	if !s.fresh(staleAfter) {
		return unknownLagBytes
	}

	if s.position <= replay {
		return 0
	}

	// A lag wider than 2^63 bytes is not a real PostgreSQL cluster.
	return int64(s.position - replay) //nolint:gosec
}

// replicaQualification is the outcome of applying the gate policy to one
// node. The reason itself determines eligibility, so no parallel bool can
// contradict it.
type replicaQualification uint8

const (
	replicaUnavailable replicaQualification = iota
	replicaBehind
	replicaCaughtUp
	replicaWithinLag
)

func (q replicaQualification) eligible() bool {
	return q == replicaCaughtUp || q == replicaWithinLag
}

func (q replicaQualification) reason() string {
	switch q {
	case replicaBehind:
		return metrics.ReasonBehind
	case replicaCaughtUp:
		return metrics.ReasonCaughtUp
	case replicaWithinLag:
		return metrics.ReasonWithinLag
	default:
		return metrics.ReasonNoHealthyReplica
	}
}

// replicaSelection is either a selected node or a reason no node qualified.
// Absence is represented by the missing node rather than an index sentinel.
type replicaSelection struct {
	node          *replicaNode
	qualification replicaQualification
}

func (s replicaSelection) available() bool { return s.node != nil }

func (s replicaSelection) reason() string { return s.qualification.reason() }

// routingRequest captures the policy-relevant state for one statement. The
// router asks it domain questions instead of repeatedly coordinating raw
// strategy, tracker and option flags.
type routingRequest struct {
	class           sqlclass.Class
	strategy        Strategy
	tracker         *Tracker
	noTrackerPolicy NoTrackerPolicy
	fallbackPolicy  FallbackPolicy
}

func newRoutingRequest(ctx context.Context, class sqlclass.Class, opts Options) routingRequest {
	return routingRequest{
		class:           class,
		strategy:        StrategyFromContext(ctx),
		tracker:         TrackerFromContext(ctx),
		noTrackerPolicy: opts.NoTracker,
		fallbackPolicy:  opts.Fallback,
	}
}

func (r routingRequest) isRead() bool { return r.class == sqlclass.Read }

func (r routingRequest) requiresReplica() bool { return r.strategy == StrategyReplica }

func (r routingRequest) forcesPrimary() bool { return r.strategy == StrategyPrimary }

func (r routingRequest) permitsFallback() bool {
	return !r.requiresReplica() && r.fallbackPolicy == FallbackToPrimary
}

func (r routingRequest) mustStayOnPrimary() bool {
	return r.tracker.Tainted() && r.strategy != StrategyStaleRead
}

func (r routingRequest) isUnscoped() bool {
	return r.tracker == nil && r.strategy == StrategyUnset && r.noTrackerPolicy == NoTrackerPrimary
}

func (r routingRequest) requiredPosition() wal.LSN {
	if r.strategy == StrategyStaleRead {
		return 0
	}

	return r.tracker.Watermark()
}

func (r routingRequest) error(cause error, watermark wal.LSN) error {
	return &RoutingError{Strategy: r.strategy, Class: r.class, Watermark: watermark, Err: cause}
}

func (r routingRequest) primary(pool *pgxpool.Pool, reason string) Target {
	return newPrimaryTarget(pool, r.class, r.strategy, reason)
}

func (r routingRequest) replica(selection replicaSelection) Target {
	return Target{
		pool:         selection.node.pool,
		destination:  replicaRole,
		replicaIndex: selection.node.index,
		class:        r.class,
		strategy:     r.strategy,
		reason:       selection.reason(),
	}
}

func newPrimaryTarget(pool *pgxpool.Pool, class sqlclass.Class, strategy Strategy, reason string) Target {
	return Target{pool: pool, destination: primaryRole, class: class, strategy: strategy, reason: reason}
}

// transactionTarget keeps the selected pool and its role inseparable.
type transactionTarget struct {
	pool *pgxpool.Pool
	role targetRole
}

type targetRole uint8

const (
	primaryRole targetRole = iota
	replicaRole
)

func primaryTransaction(pool *pgxpool.Pool) transactionTarget {
	return transactionTarget{pool: pool, role: primaryRole}
}

func transactionFrom(target Target) transactionTarget {
	return transactionTarget{pool: target.pool, role: target.destination}
}

func (t transactionTarget) onPrimary() bool { return t.role == primaryRole }
