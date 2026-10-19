package uow

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type ctxKey struct{}

// FromContext returns the pgx.Tx from ctx, or nil if not set.
func FromContext(ctx context.Context) pgx.Tx { //nolint:ireturn // public API returns the pgx.Tx interface
	tx, ok := ctx.Value(ctxKey{}).(pgx.Tx)
	if !ok {
		return nil
	}

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
