/*
Package event_store - implementation of event store
*/
package eventsourcing

import (
	"context"
	"log/slog"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db"
	es_postgres "github.com/shortlink-org/go-sdk/eventsourcing/store/postgres"
	"github.com/shortlink-org/go-sdk/logger"
)

// New - create new EventStore
func New(ctx context.Context, log logger.Logger, store db.DB, cfg *config.Config) (EventSourcing, error) {
	var err error

	// Initialize EventStore
	e := &eventSourcing{cfg: cfg}

	// Set configuration
	e.setConfig()

	switch e.typeStore {
	case "postgres":
		e.repository, err = es_postgres.New(ctx, store)
		if err != nil {
			return nil, err
		}
	default:
		e.repository, err = es_postgres.New(ctx, store)
		if err != nil {
			return nil, err
		}
	}

	log.Info("run db",
		slog.String("db", e.typeStore),
	)

	return e.repository, nil
}

// setConfig - set configuration
func (e *eventSourcing) setConfig() {
	e.cfg.SetDefault("STORE_TYPE", "ram") // Select: postgres

	e.typeStore = e.cfg.GetString("STORE_TYPE")
}
