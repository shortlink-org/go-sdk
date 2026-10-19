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
func New(ctx context.Context, log *slog.Logger, store db.DB, cfg *config.Config) (EventSourcing, error) { //nolint:ireturn // public API returns the interface
	var err error

	// Initialize EventStore
	eventStore := &eventSourcing{cfg: cfg}

	// Set configuration
	eventStore.setConfig()

	switch eventStore.typeStore {
	case "postgres":
		eventStore.repository, err = es_postgres.New(ctx, store)
		if err != nil {
			return nil, err
		}
	default:
		eventStore.repository, err = es_postgres.New(ctx, store)
		if err != nil {
			return nil, err
		}
	}

	log.Info("run db",
		slog.String("db", eventStore.typeStore),
	)

	return eventStore.repository, nil
}

// setConfig - set configuration
func (e *eventSourcing) setConfig() {
	e.cfg.SetDefault("STORE_TYPE", "ram") // Select: postgres

	e.typeStore = e.cfg.GetString("STORE_TYPE")
}
