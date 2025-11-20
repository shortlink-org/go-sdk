/*
Data Base package
*/
package db

import (
	"context"
	"log/slog"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db/drivers/badger"
	"github.com/shortlink-org/go-sdk/db/drivers/cockroachdb"
	"github.com/shortlink-org/go-sdk/db/drivers/dgraph"
	"github.com/shortlink-org/go-sdk/db/drivers/leveldb"
	"github.com/shortlink-org/go-sdk/db/drivers/mongo"
	"github.com/shortlink-org/go-sdk/db/drivers/mysql"
	"github.com/shortlink-org/go-sdk/db/drivers/neo4j"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres"
	"github.com/shortlink-org/go-sdk/db/drivers/ram"
	"github.com/shortlink-org/go-sdk/db/drivers/redis"
	"github.com/shortlink-org/go-sdk/db/drivers/sqlite"
	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
)

// New - return implementation of db
func New(ctx context.Context, log logger.Logger, tracer trace.TracerProvider, metrics *metric.MeterProvider, cfg *config.Config) (DB, error) {
	//nolint:exhaustruct // fix later, use constructor
	store := &Store{
		cfg: cfg,
	}

	// Set configuration
	store.setConfig()

	switch store.typeStore {
	case "cockroachdb":
		store.DB = cockroachdb.New(cfg)
	case "postgres":
		store.DB = postgres.New(tracer, metrics, cfg)
	case "mysql":
		store.DB = mysql.New(tracer, metrics, cfg)
	case "mongo":
		store.DB = mongo.New(cfg)
	case "redis":
		store.DB = redis.New(tracer, metrics, cfg)
	case "dgraph":
		store.DB = dgraph.New(log, cfg)
	case "leveldb":
		store.DB = leveldb.New(cfg)
	case "badger":
		store.DB = badger.New(cfg)
	case "ram":
		store.DB = ram.New(cfg)
	case "neo4j":
		store.DB = neo4j.New(cfg)
	case "sqlite":
		store.DB = sqlite.New(tracer, metrics, cfg)
	default:
		store.DB = ram.New(cfg)
	}

	if err := store.Init(ctx); err != nil {
		return nil, err
	}

	log.Info("run db",
		slog.String("db", store.typeStore),
	)

	return store, nil
}

// setConfig - set configuration
func (s *Store) setConfig() {
	s.cfg.SetDefault("STORE_TYPE", "ram") // Select: postgres, mysql, mongo, redis, dgraph, sqlite, leveldb, badger, neo4j, ram, cockroachdb

	s.typeStore = s.cfg.GetString("STORE_TYPE")
	if s.typeStore == "" {
		s.typeStore = "ram"
	}
}
