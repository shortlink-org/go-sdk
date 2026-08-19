package replica

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/metrics"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// watermarkSQL reads the primary's current WAL insert position.
//
// Insert rather than flush or current: it names a position at or ahead of our
// commit record, never behind it. Being ahead only makes a replica look less
// caught up than it is, which costs throughput; being behind would let a gate
// open one record too early, which costs correctness.
//
// The cast to text is not optional — pgx has no codec for pg_lsn.
const watermarkSQL = `SELECT pg_current_wal_insert_lsn()::text`

// Target is where a statement would run, and why.
type Target struct {
	Pool *pgxpool.Pool
	// Replica is the replica index, or -1 for the primary.
	Replica  int
	Class    sqlclass.Class
	Strategy Strategy
	Reason   string
}

// OnPrimary reports whether the statement would run on the primary.
func (t Target) OnPrimary() bool { return t.Replica < 0 }

// Router splits statements between the primary and its replicas.
//
// It implements the part of *pgxpool.Pool that repository code uses, so
// adopting it is usually a change of one variable's type. The methods it does
// not carry over — Acquire, Stat, Config — have genuinely different semantics
// across several pools and are documented individually.
//
// The zero value is not usable; get one from RouterFrom.
type Router struct {
	primary *pgxpool.Pool
	gate    *gate
	metrics *metrics.Metrics
	opts    Options
}

// Enabled reports whether any replica is configured. A disabled router is
// still safe to use: every statement goes to the primary.
func (r *Router) Enabled() bool {
	return r != nil && r.gate != nil && len(r.gate.replicas) > 0
}

// Primary returns the primary pool.
func (r *Router) Primary() *pgxpool.Pool {
	return r.primary
}

// Replicas returns the replica pools, in configuration order.
func (r *Router) Replicas() []*pgxpool.Pool {
	if !r.Enabled() {
		return nil
	}

	pools := make([]*pgxpool.Pool, 0, len(r.gate.replicas))
	for _, node := range r.gate.replicas {
		pools = append(pools, node.pool)
	}

	return pools
}

// Health returns what the poller last saw for each replica. It never queries
// the database.
func (r *Router) Health() []ReplicaHealth {
	if !r.Enabled() {
		return nil
	}

	return r.gate.health()
}

// MarkWritten records that a write happened on this context through some path
// the router did not see — a second pool, a raw database/sql handle, an ORM.
//
// Without it such a write is invisible: the context is not tainted, and the
// next read is served from a replica that may not have replayed it.
func (r *Router) MarkWritten(ctx context.Context) {
	TrackerFromContext(ctx).Taint()
}

// Watermark reads the primary's current WAL position, and records it on the
// context's tracker.
//
// Call it when handing a read-after-write guarantee across a process
// boundary — an HTTP response the client will follow up on, a message another
// service will consume. Within one request it is unnecessary: the taint
// already keeps subsequent reads on the primary.
//
// It costs one round trip, which is why it is explicit rather than automatic.
func (r *Router) Watermark(ctx context.Context) (wal.LSN, error) {
	if r == nil || r.primary == nil {
		return 0, ErrRouterDisabled
	}

	started := time.Now()

	var text string

	err := r.primary.QueryRow(ctx, watermarkSQL).Scan(&text)

	r.metrics.CaptureDuration(ctx, metrics.CaptureStandalone, time.Since(started), err)

	if err != nil {
		return 0, &Error{Op: opWatermark, Err: err, Details: "failed to read the primary WAL position"}
	}

	position, err := wal.ParseLSN(text)
	if err != nil {
		return 0, &Error{Op: opWatermark, Err: err, Details: "unparseable WAL position"}
	}

	r.gate.observePrimary(position)
	TrackerFromContext(ctx).Observe(position)

	return position, nil
}

// Token wraps Watermark with the cluster lineage, producing a value that is
// safe to send across a process boundary.
//
// The lineage is what makes it safe. After a failover an LSN from the previous
// timeline names a byte offset that either never existed on the new primary or
// now holds unrelated records, so comparing it against the new replay position
// is meaningless in both directions: it can pin every read to the primary
// forever, or report "caught up" for content that never replicated.
func (r *Router) Token(ctx context.Context) (wal.Token, error) {
	position, err := r.Watermark(ctx)
	if err != nil {
		return wal.Token{}, err
	}

	systemID, timeline, _ := r.gate.lineage()

	return wal.Token{
		SystemID: systemID,
		Timeline: timeline,
		LSN:      position,
		IssuedAt: time.Now(),
	}, nil
}

// Accept turns a token received from elsewhere into a position this process
// may act on, and reports whether it could be interpreted at all.
//
// A false result does not mean "no watermark". It means the token could not be
// compared to anything here, and treating that as an absent watermark would
// serve exactly the stale read the token exists to prevent — so the caller must
// pin the read to the primary instead. WithToken does that for you.
//
// A position beyond what the primary is known to have written is clamped rather
// than refused. It arises two ways: a forged header, and a token minted on
// another pod within the last poll interval. Clamping handles both — the read
// waits for the replica to reach the primary's last known position, which is
// bounded by ordinary replication lag, so a crafted value buys an attacker
// nothing and an honest token is not thrown away.
func (r *Router) Accept(token wal.Token) (wal.LSN, bool) {
	if token.IsZero() {
		return 0, false
	}

	systemID, timeline, known := r.gate.lineage()
	if known {
		// A zero system identifier means the role could not read
		// pg_control_system(); fall back to comparing timelines alone, which
		// still catches every failover.
		if token.Timeline != timeline {
			return 0, false
		}

		if systemID != 0 && token.SystemID != 0 && token.SystemID != systemID {
			return 0, false
		}
	}

	if primary, ok := r.gate.primaryPosition(); ok && token.LSN > primary {
		return primary, true
	}

	return token.LSN, true
}

// WithToken installs the guarantee a token carries onto ctx.
//
// It is the safe way to consume a token, and the one the boundary middlewares
// use. An uninterpretable token pins the context to the primary rather than
// being dropped: after a failover, a token from the previous timeline says
// "this reader has already seen a write" just as loudly as one we can compare,
// and the only thing we no longer know is which replica has caught up.
func (r *Router) WithToken(ctx context.Context, token wal.Token) context.Context {
	if token.IsZero() {
		return ctx
	}

	position, ok := r.Accept(token)
	if !ok {
		return OnPrimary(ctx)
	}

	return WithWatermark(ctx, position)
}

// Ready reports whether some replica has already replayed the given position.
// It consults the poller's cached sample and never queries the database.
func (r *Router) Ready(required wal.LSN) bool {
	if !r.Enabled() {
		return false
	}

	return r.gate.ready(required)
}

// Await waits, up to the configured gate budget, for a replica to replay the
// given position. It reports whether one did.
//
// The queue consumer uses it: waiting briefly beats nacking, because a nack
// feeds the retry middleware and a message that is merely early can exhaust
// its retries and land in the dead-letter queue as though it were malformed.
func (r *Router) Await(ctx context.Context, required wal.LSN) (bool, error) {
	if !r.Enabled() {
		return true, nil
	}

	return r.gate.await(ctx, required, r.opts.GateMaxWait)
}

// Route reports where a statement would run, without running it. It exists for
// tests, for logging, and for callers assembling their own execution path.
func (r *Router) Route(ctx context.Context, sql string) (Target, error) {
	class := r.opts.Classifier.Classify(sql)

	return r.route(ctx, class)
}

// route is the whole decision, and it makes no I/O: everything it consults is
// either in the context or in the poller's cached sample.
func (r *Router) route(ctx context.Context, class sqlclass.Class) (Target, error) {
	strategy := StrategyFromContext(ctx)
	tracker := TrackerFromContext(ctx)

	target := Target{
		Pool:     r.primary,
		Replica:  -1,
		Class:    class,
		Strategy: strategy,
	}

	// A write is a write. The strategy may not override that: promoting it to
	// the primary silently would make StrategyReplica a lie, so say so.
	if class != sqlclass.Read {
		if strategy == StrategyReplica {
			return target, &RoutingError{
				Strategy:  strategy,
				Class:     class,
				Watermark: tracker.Watermark(),
				Err:       ErrWriteOnReplica,
			}
		}

		tracker.Taint()

		target.Reason = writeReason(class)

		return target, nil
	}

	if strategy == StrategyPrimary {
		target.Reason = metrics.ReasonExplicit

		return target, nil
	}

	if !r.Enabled() {
		if strategy == StrategyReplica {
			return target, &RoutingError{Strategy: strategy, Class: class, Err: ErrRouterDisabled}
		}

		target.Reason = metrics.ReasonRouterDisabled

		return target, nil
	}

	// A tainted context has written without a known position. Nothing a
	// replica can report proves it has caught up, so there is nothing to
	// compare and the primary is the only correct answer.
	if tracker.Tainted() && strategy != StrategyStaleRead {
		if strategy == StrategyReplica {
			return target, &RoutingError{
				Strategy:  strategy,
				Class:     class,
				Watermark: tracker.Watermark(),
				Err:       ErrNoHealthyReplica,
			}
		}

		target.Reason = metrics.ReasonTainted

		return target, nil
	}

	// No tracker and no explicit strategy: nobody has scoped this read. The
	// default sends it to the primary, because an unscoped read is one nobody
	// has reasoned about and a stale row surfaces as a bug somewhere else.
	if tracker == nil && strategy == StrategyUnset && r.opts.NoTracker == NoTrackerPrimary {
		target.Reason = metrics.ReasonNoTracker

		return target, nil
	}

	required := tracker.Watermark()
	if strategy == StrategyStaleRead {
		required = 0
	}

	index, reason := r.gate.pick(required)
	if index < 0 {
		if strategy == StrategyReplica || !r.opts.Fallback {
			return target, &RoutingError{
				Strategy:  strategy,
				Class:     class,
				Watermark: required,
				Err:       ErrNoHealthyReplica,
			}
		}

		target.Reason = reason

		return target, nil
	}

	target.Pool = r.gate.replicas[index].pool
	target.Replica = index
	target.Reason = reason

	return target, nil
}

// pick resolves a target and counts it. Every routed method starts here.
func (r *Router) pick(ctx context.Context, sql string) (Target, error) {
	target, err := r.route(ctx, r.opts.Classifier.Classify(sql))
	if err != nil {
		return target, err
	}

	r.metrics.RecordDecision(ctx, decisionOf(target))

	return target, nil
}

// tx returns the application-managed transaction carried by ctx, if the
// WithTxLookup hook was wired.
//
//nolint:ireturn // returns whatever pgx.Tx the application put in the context
func (r *Router) tx(ctx context.Context) pgx.Tx {
	if r.opts.TxLookup == nil {
		return nil
	}

	return r.opts.TxLookup(ctx)
}

// writeReason maps a non-read class onto its metric attribute.
func writeReason(class sqlclass.Class) string {
	if class == sqlclass.Write {
		return metrics.ReasonWrite
	}

	return metrics.ReasonUnknown
}

// decisionOf reduces a routing target to the attributes it is counted by.
func decisionOf(target Target) metrics.Decision {
	name := metrics.TargetReplica
	if target.OnPrimary() {
		name = metrics.TargetPrimary
	}

	return metrics.Decision{
		Target:   name,
		Reason:   target.Reason,
		Class:    target.Class.String(),
		Strategy: target.Strategy.String(),

		// A read that asked for a replica and got the primary is the signal
		// that tells you whether the feature is paying for itself.
		Fallback: target.OnPrimary() && target.Class == sqlclass.Read,
	}
}

// New builds a router over an already-open primary and its replicas.
