package db

import (
	"context"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres"
)

// DB - common interface of db
type DB interface {
	Init(ctx context.Context) error
	GetConn() any
}

// Store abstract type
type Store struct {
	DB

	typeStore string
	cfg       *config.Config
}

// Options holds configuration options for database initialization.
type Options struct {
	// PostgresOptions are options specific to PostgreSQL driver.
	PostgresOptions []postgres.Option
}

// Option is a functional option for configuring database.
type Option func(*Options)

// WithPostgresAfterConnect sets the AfterConnect callback for PostgreSQL.
func WithPostgresAfterConnect(fn postgres.AfterConnectFunc) Option {
	return func(o *Options) {
		o.PostgresOptions = append(o.PostgresOptions, postgres.WithAfterConnect(fn))
	}
}
