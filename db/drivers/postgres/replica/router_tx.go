package replica

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// replicaEligible reports whether a transaction's options prove it cannot
// write, so it may run on a standby.
//
// Each clause earns its place:
//
//   - AccessMode == pgx.ReadOnly is the caller's explicit, machine-checkable
//     promise, and the server enforces it too — a caller who lies gets an
//     error rather than silent divergence.
//
//   - BeginQuery and CommitQuery must be empty. pgx emits BeginQuery verbatim
//     and ignores AccessMode entirely when it is set, so
//     TxOptions{AccessMode: ReadOnly, BeginQuery: "BEGIN"} is a read-write
//     transaction that inspects as read-only. Silent, and a real trap.
//
//   - Serializable is unavailable on a hot standby. Routing a serializable
//     read-only transaction to a replica turns working code into a runtime
//     error, and only in production, where the replica exists.
//
//nolint:gocritic // pgx.TxOptions is passed by value throughout pgx's own API
func replicaEligible(options pgx.TxOptions) bool {
	return options.AccessMode == pgx.ReadOnly &&
		options.BeginQuery == "" &&
		options.CommitQuery == "" &&
		options.IsoLevel != pgx.Serializable
}

// routedTx keeps the routing decision attached to a transaction.
//
// It embeds pgx.Tx and overrides only what it must. Begin has to be
// overridden because pgxpool's Tx returns the inner pgx.Tx directly, so a
// nested savepoint would otherwise escape the wrapper and lose the tracking.
//
// Conn is inherited and hands out the raw connection. That is a hole, and an
// unavoidable one: a caller holding a *pgx.Conn can do anything on it. Using
// it opts out of routing entirely, exactly like Acquire.
type routedTx struct {
	pgx.Tx

	router    *Router
	onPrimary bool
}

var _ pgx.Tx = (*routedTx)(nil)

// Begin starts a nested transaction (a savepoint), keeping the wrapper.
//
//nolint:ireturn // implements pgx.Tx
func (t *routedTx) Begin(ctx context.Context) (pgx.Tx, error) {
	nested, err := t.Tx.Begin(ctx)
	if err != nil {
		return nil, err
	}

	return &routedTx{Tx: nested, router: t.router, onPrimary: t.onPrimary}, nil
}

// Commit commits the transaction and, when WithSyncWatermark is on, resolves
// the WAL position the context must now observe.
//
// The position is read after the commit and on the pool, not on the
// transaction's own connection: pgxpool releases that connection during
// Commit, so tx.Conn() is invalid by the time this runs.
//
// A failure to resolve it records an unknown position rather than nothing. Degrading to
// the primary is the point; degrading to "probably fine" is how this feature
// would quietly stop working.
func (t *routedTx) Commit(ctx context.Context) error {
	err := t.Tx.Commit(ctx)
	if err != nil {
		return err
	}

	if !t.onPrimary || !t.router.opts.SyncWatermark {
		return nil
	}

	tracker := TrackerFromContext(ctx)
	if tracker == nil {
		return nil
	}

	position, watermarkErr := t.router.Watermark(ctx)
	if watermarkErr != nil {
		// The commit landed; only the position is unknown. Reporting the
		// failure would roll back a transaction that is already durable, so
		// degrade the guarantee instead.
		tracker.Observe(wal.Unknown)

		return nil //nolint:nilerr // the commit succeeded; only the watermark did not
	}

	tracker.Observe(position)

	return nil
}
