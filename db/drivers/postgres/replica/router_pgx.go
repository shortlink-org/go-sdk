package replica

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/metrics"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass"
)

// Pool is the part of *pgxpool.Pool that repository code actually uses. A
// repository written against it takes a Router with no other change.
//
// Acquire, Close, Reset, Stat and Config are deliberately absent: across
// several pools their semantics genuinely differ, so a caller that needs one
// should have to think about which pool it means.
type Pool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults
	CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, source pgx.CopyFromSource) (int64, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error)
	Ping(ctx context.Context) error
}

var (
	_ Pool = (*pgxpool.Pool)(nil)
	_ Pool = (*Router)(nil)
)

// Exec runs a statement. It classifies rather than assuming: Exec is not a
// synonym for "write", and one rule for every method is far easier to reason
// about than a per-method exception.
func (r *Router) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if tx := r.tx(ctx); tx != nil {
		r.inTransaction(ctx)

		return tx.Exec(ctx, sql, args...)
	}

	target, err := r.pick(ctx, sql)
	if err != nil {
		return pgconn.CommandTag{}, err
	}

	tag, err := target.Pool.Exec(ctx, sql, args...)
	if err != nil && r.shouldRetryOnPrimary(ctx, target, err) {
		return r.primary.Exec(ctx, sql, args...)
	}

	return tag, err
}

// Query runs a statement and returns its rows.
//
// It is the reason the classifier exists: Query is routinely used for
// INSERT ... RETURNING, which is why matching on a SELECT prefix is not enough.
//
//nolint:ireturn // pgx.Rows is the type pgxpool.Pool returns; this is a drop-in
func (r *Router) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if tx := r.tx(ctx); tx != nil {
		r.inTransaction(ctx)

		return tx.Query(ctx, sql, args...)
	}

	target, err := r.pick(ctx, sql)
	if err != nil {
		return errRows{err: err}, err
	}

	rows, err := target.Pool.Query(ctx, sql, args...)
	if err != nil && r.shouldRetryOnPrimary(ctx, target, err) {
		return r.primary.Query(ctx, sql, args...)
	}

	return rows, err
}

// QueryRow runs a statement and returns its first row.
//
// A routing failure reaches the caller through Row.Scan, because that is the
// only channel this signature has. It is not silently downgraded to a run on
// the primary: a caller who asked for a replica and got something else should
// find out.
//
//nolint:ireturn // pgx.Row is the type pgxpool.Pool returns; this is a drop-in
func (r *Router) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx := r.tx(ctx); tx != nil {
		r.inTransaction(ctx)

		return tx.QueryRow(ctx, sql, args...)
	}

	target, err := r.pick(ctx, sql)
	if err != nil {
		return errRow{err: err}
	}

	return target.Pool.QueryRow(ctx, sql, args...)
}

// SendBatch pipelines several statements down one connection.
//
// A batch cannot be split, so it goes to a replica only when every statement
// in it is a read. Mixed batches — an INSERT and a SELECT in one round trip —
// are common, and they go to the primary, which is no worse than today.
//
// The taint is applied here, at queue-inspection time, and not when the
// results are closed. BatchResults is lazy: a caller that queues a write and
// then does an unrelated read before closing the batch would otherwise get
// that read served from a replica.
//
//nolint:ireturn // pgx.BatchResults is the type pgxpool.Pool returns
func (r *Router) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	if tx := r.tx(ctx); tx != nil {
		r.inTransaction(ctx)

		return tx.SendBatch(ctx, batch)
	}

	target, err := r.route(ctx, r.classifyBatch(batch))
	if err != nil {
		return errBatchResults{err: err}
	}

	r.metrics.RecordDecision(ctx, decisionOf(target))

	return target.Pool.SendBatch(ctx, batch)
}

// classifyBatch reduces a batch to the most restrictive class it contains. An
// empty batch counts as unknown: there is nothing to gain by sending it to a
// replica and nothing to lose by not.
func (r *Router) classifyBatch(batch *pgx.Batch) sqlclass.Class {
	if batch == nil || len(batch.QueuedQueries) == 0 {
		return sqlclass.Unknown
	}

	for _, queued := range batch.QueuedQueries {
		if r.opts.Classifier.Classify(queued.SQL) != sqlclass.Read {
			return sqlclass.Write
		}
	}

	return sqlclass.Read
}

// CopyFrom bulk-loads rows. Always the primary, always tainting: COPY FROM is
// an insert by construction, and there is no statement text to classify.
func (r *Router) CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, source pgx.CopyFromSource) (int64, error) {
	if tx := r.tx(ctx); tx != nil {
		r.inTransaction(ctx)

		return tx.CopyFrom(ctx, table, columns, source)
	}

	TrackerFromContext(ctx).Taint()
	r.metrics.RecordDecision(ctx, decisionOf(Target{Pool: r.primary, Replica: -1, Class: sqlclass.Write, Reason: metrics.ReasonWrite}))

	return r.primary.CopyFrom(ctx, table, columns, source)
}

// Begin starts a transaction on the primary and taints the context.
//
// The driver implements Begin as BeginTx with zero options, and a zero
// TxOptions emits a bare "begin" — read-write under the server's defaults. The
// statements inside are unknown at this point, and by the time we learn the
// transaction writes, its earlier reads have already happened. Pessimism here
// is the only sound rule; use BeginTx with pgx.ReadOnly to say otherwise.
//
//nolint:ireturn // pgx.Tx is the type pgxpool.Pool returns
func (r *Router) Begin(ctx context.Context) (pgx.Tx, error) {
	return r.BeginTx(ctx, pgx.TxOptions{})
}

// BeginTx starts a transaction, on a replica when the options prove it is
// read-only and the strategy allows it, and on the primary otherwise.
//
//nolint:ireturn,gocritic // pgx.Tx matches pgxpool; TxOptions by value matches pgx's own signature
func (r *Router) BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	if !replicaEligible(options) {
		TrackerFromContext(ctx).Taint()
		r.metrics.RecordDecision(ctx, decisionOf(Target{Pool: r.primary, Replica: -1, Class: sqlclass.Write, Reason: metrics.ReasonWrite}))

		tx, err := r.primary.BeginTx(ctx, options)
		if err != nil {
			return nil, err
		}

		return &routedTx{Tx: tx, router: r, onPrimary: true}, nil
	}

	target, err := r.route(ctx, sqlclass.Read)
	if err != nil {
		return nil, err
	}

	r.metrics.RecordDecision(ctx, decisionOf(target))

	// A transaction that lands on the primary is presumed to write, even when
	// its options say read-only: the caller may still be using it to serialize
	// work, and there is nothing to gain from being clever here.
	if target.OnPrimary() {
		TrackerFromContext(ctx).Taint()
	}

	tx, err := target.Pool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}

	return &routedTx{Tx: tx, router: r, onPrimary: target.OnPrimary()}, nil
}

// Ping checks the primary.
//
// The write path being up is the meaningful signal at startup; a boot that
// fails because one replica is slow to come up is a worse outage than a
// degraded read path. Use PingAll for the whole picture.
func (r *Router) Ping(ctx context.Context) error {
	return r.primary.Ping(ctx)
}

// PingAll pings the primary and every replica, joining the failures. A replica
// being down does not make the router unusable.
func (r *Router) PingAll(ctx context.Context) error {
	replicas := r.Replicas()

	errs := make([]error, 0, len(replicas)+1)
	errs = append(errs, r.primary.Ping(ctx))

	for _, pool := range replicas {
		errs = append(errs, pool.Ping(ctx))
	}

	return errors.Join(errs...)
}

// Acquire takes a connection from the primary and taints the context.
//
// Statements run on the returned connection are invisible to the router: it
// cannot classify them and cannot see what they wrote. The primary plus a
// taint is the only choice that cannot be wrong, which makes Acquire the
// deliberate escape hatch. For a connection you promise to use read-only, ask
// for one explicitly with AcquireReplica.
func (r *Router) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
	TrackerFromContext(ctx).Taint()

	return r.primary.Acquire(ctx)
}

// AcquireReplica takes a connection from a replica chosen by the context's
// strategy. The caller promises to issue only read-only statements; the router
// cannot check. It reports ErrNoHealthyReplica rather than falling back, since
// a caller asking for this has already decided what it wants.
func (r *Router) AcquireReplica(ctx context.Context) (*pgxpool.Conn, error) {
	target, err := r.route(OnReplica(ctx), sqlclass.Read)
	if err != nil {
		return nil, err
	}

	return target.Pool.Acquire(ctx)
}

// Close closes every pool and stops the poller.
func (r *Router) Close() {
	if r.gate != nil {
		r.gate.close()
	}

	r.primary.Close()
}

// Reset closes every idle connection in every pool. A network interruption or
// a server state change affects them all.
func (r *Router) Reset() {
	r.primary.Reset()

	for _, pool := range r.Replicas() {
		pool.Reset()
	}
}

// Stat returns the primary's pool statistics.
//
// It cannot merge across pools: pgxpool.Stat has only unexported fields, so a
// combined value cannot be constructed from outside that package. Wiring this
// straight into a dashboard therefore under-reports total connections once
// replicas exist — use Stats for the full picture.
func (r *Router) Stat() *pgxpool.Stat {
	return r.primary.Stat()
}

// Stats returns statistics for every pool, the primary first.
func (r *Router) Stats() []*pgxpool.Stat {
	replicas := r.Replicas()

	stats := make([]*pgxpool.Stat, 0, len(replicas)+1)
	stats = append(stats, r.primary.Stat())

	for _, pool := range replicas {
		stats = append(stats, pool.Stat())
	}

	return stats
}

// Config returns a copy of the primary's pool configuration.
func (r *Router) Config() *pgxpool.Config {
	return r.primary.Config()
}

// inTransaction records that the statement ran inside a caller-managed
// transaction, which is on the primary by construction.
func (r *Router) inTransaction(ctx context.Context) {
	TrackerFromContext(ctx).Taint()
	r.metrics.RecordDecision(ctx, decisionOf(Target{
		Pool:    r.primary,
		Replica: -1,
		Class:   sqlclass.Write,
		Reason:  metrics.ReasonInTransaction,
	}))
}

// shouldRetryOnPrimary reports whether a failed replica statement may be run
// again on the primary.
//
// The rule is narrow on purpose: only a statement that provably never reached
// the server. The pgconn package has a predicate for exactly that,
// SafeToRetry, and deferring to it keeps this correct across pgx upgrades.
//
// It never applies to results read lazily. By the time a replica failure
// surfaces through Rows.Next the caller may already have scanned rows and
// appended them somewhere; re-running the query would duplicate them with no
// way for anyone to notice.
func (r *Router) shouldRetryOnPrimary(ctx context.Context, target Target, err error) bool {
	if target.OnPrimary() || !r.opts.Fallback || !pgconn.SafeToRetry(err) {
		return false
	}

	r.gate.markFailure(target.Replica, err)
	r.metrics.Fallback(ctx, metrics.ReasonSafeToRetry)

	return true
}
