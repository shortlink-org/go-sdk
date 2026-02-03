package uow

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type ctxKey struct{}

// FromContext returns the pgx.Tx from ctx, or nil if not set.
func FromContext(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(ctxKey{}).(pgx.Tx)
	return tx
}

// WithTx returns a context that carries the given transaction.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

// HasTx reports whether ctx contains a transaction.
func HasTx(ctx context.Context) bool {
	return FromContext(ctx) != nil
}
