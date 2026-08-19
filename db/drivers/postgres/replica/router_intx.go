package replica

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/metrics"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// InTx runs fn inside a transaction on the pool the context's strategy selects,
// and owns the whole lifecycle: rollback on error, rollback on panic, and
// exactly one release of the connection.
//
// It exists for one reason BeginTx cannot serve. With WithSyncWatermark on, the
// WAL position has to be read after the commit — read inside the transaction it
// excludes the commit record and the gate opens one record too early. Through
// pgx.Tx that is necessarily a second round trip, because Commit sends COMMIT
// on its own. Here the two are pipelined into one:
//
//	commit; select pg_current_wal_insert_lsn()::text
//
// On this machine that saves about 110µs per commit; on a real network, one
// round trip.
//
// The transaction passed to work is valid only for the duration of the call. Do
// not retain it, and do not commit or roll it back yourself.
//
//nolint:gocritic // pgx.TxOptions is passed by value throughout pgx's own API
func (r *Router) InTx(ctx context.Context, options pgx.TxOptions, work func(context.Context, pgx.Tx) error) error {
	pool, onPrimary, err := r.txPool(ctx, options)
	if err != nil {
		return err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return &Error{Op: opInTx, Err: err, Details: "failed to acquire a connection"}
	}

	defer conn.Release()

	// The transaction is started on the raw connection rather than on the
	// pool, so that committing does not also release the connection behind our
	// back — this function owns the release.
	transaction, err := conn.Conn().BeginTx(ctx, options)
	if err != nil {
		return &Error{Op: opInTx, Err: err, Details: "failed to begin"}
	}

	committed := false

	defer func() {
		// A stray ROLLBACK after a successful commit would run outside any
		// transaction, so it is guarded rather than deferred unconditionally.
		if !committed {
			//nolint:errcheck // the caller already has the reason this is unwinding
			_ = transaction.Rollback(ctx)
		}
	}()

	err = work(ctx, transaction)
	if err != nil {
		return err
	}

	if !onPrimary || !r.opts.SyncWatermark {
		err = transaction.Commit(ctx)
		if err != nil {
			return err
		}

		committed = true

		return nil
	}

	position, err := r.commitWithWatermark(ctx, conn, commitStatement(options))
	if err != nil {
		return err
	}

	committed = true

	TrackerFromContext(ctx).Observe(position)

	return nil
}

// txPool resolves which pool the transaction runs on, applying the same rules
// as BeginTx.
//
//nolint:gocritic // pgx.TxOptions is passed by value throughout pgx's own API
func (r *Router) txPool(ctx context.Context, options pgx.TxOptions) (*pgxpool.Pool, bool, error) {
	if !replicaEligible(options) {
		TrackerFromContext(ctx).Taint()
		r.metrics.RecordDecision(ctx, decisionOf(Target{Pool: r.primary, Replica: -1, Class: sqlclass.Write, Reason: metrics.ReasonWrite}))

		return r.primary, true, nil
	}

	target, err := r.route(ctx, sqlclass.Read)
	if err != nil {
		return nil, false, err
	}

	r.metrics.RecordDecision(ctx, decisionOf(target))

	if target.OnPrimary() {
		TrackerFromContext(ctx).Taint()
	}

	return target.Pool, target.OnPrimary(), nil
}

// commitWithWatermark commits and reads the resulting WAL position in a single
// round trip, using the simple protocol's multi-statement form.
//
// This is the one place the driver steps around pgx's transaction type. The
// pgx.Tx handed to fn is left believing it is open, which is safe only because
// InTx owns it: the guarded rollback above never runs after this succeeds, and
// the caller was told not to retain the transaction.
func (r *Router) commitWithWatermark(ctx context.Context, conn *pgxpool.Conn, commitSQL string) (wal.LSN, error) {
	multi := conn.Conn().PgConn().Exec(ctx, commitSQL+"; "+watermarkSQL)

	var (
		raw  []byte
		errs []error
	)

	for multi.NextResult() {
		reader := multi.ResultReader()

		for reader.NextRow() {
			values := reader.Values()
			if len(values) == 1 && values[0] != nil {
				// Values reuses its buffer, so the bytes have to be copied
				// before the next call moves on.
				raw = append(raw[:0], values[0]...)
			}
		}

		_, err := reader.Close()
		errs = append(errs, err)
	}

	errs = append(errs, multi.Close())

	err := errors.Join(errs...)
	if err != nil {
		// The connection's transaction state is now unknown — the commit may
		// or may not have landed. Discard the connection rather than return it
		// to the pool mid-transaction; pgxpool destroys a closed connection on
		// release.
		//nolint:errcheck // the connection is being discarded either way
		_ = conn.Conn().Close(ctx)

		return 0, &Error{Op: opInTx, Err: err, Details: "failed to commit and read the WAL position"}
	}

	if len(raw) == 0 {
		// Committed, but the position did not come back. Degrade to the
		// primary rather than to "probably fine".
		return wal.Unknown, nil
	}

	position, err := wal.ParseLSN(string(raw))
	if err != nil {
		return wal.Unknown, nil //nolint:nilerr // an unreadable position is a degraded one, not a failed commit
	}

	return position, nil
}

// commitStatement honors a caller-supplied commit query, matching what pgx
// itself would send.
//
//nolint:gocritic // pgx.TxOptions is passed by value throughout pgx's own API
func commitStatement(options pgx.TxOptions) string {
	if options.CommitQuery != "" {
		return options.CommitQuery
	}

	return "commit"
}
