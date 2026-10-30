//go:build unit

package eventsourcing_test

import "github.com/jackc/pgx/v5"

// fakeTx only marks a context as carrying a transaction.
type fakeTx struct {
	pgx.Tx
}
