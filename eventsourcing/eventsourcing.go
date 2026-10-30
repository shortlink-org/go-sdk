/*
Package eventsourcing - implementation of event store
*/
package eventsourcing

import (
	"context"
	"log/slog"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db"
	es_postgres "github.com/shortlink-org/go-sdk/eventsourcing/store/postgres"
)

// New - create new EventStore
//
// The engine follows store, and PostgreSQL is the only one: any other driver
// yields db.ErrGetConnection. The config is no longer read and stays for
// compatibility.
//
//nolint:ireturn,iface // public API returns the interface
func New(ctx context.Context, log *slog.Logger, store db.DB, _ *config.Config, opts ...es_postgres.Option) (EventSourcing, error) {
	repository, err := es_postgres.New(ctx, store, opts...)
	if err != nil {
		return nil, err
	}

	if log != nil {
		log.Info("run db",
			slog.String("db", "postgres"),
		)
	}

	return repository, nil
}
