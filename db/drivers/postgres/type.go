package postgres

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/sdk/metric"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica"
)

// AfterConnectFunc is a callback executed after each new connection is established.
type AfterConnectFunc func(ctx context.Context, conn *pgx.Conn) error

// Config - config
type Config struct {
	mode   int
	config *pgxpool.Config
}

// Option is a functional option for Store configuration.
//
// Options are applied in Init, after the defaults and the configuration keys,
// so an option always wins over an environment variable.
type Option func(*Store)

// WithAfterConnect sets a callback to be executed after each new connection.
// Useful for registering custom types (e.g., pgx-shopspring-decimal).
func WithAfterConnect(fn AfterConnectFunc) Option {
	return func(s *Store) {
		s.afterConnect = fn
	}
}

// Store implementation of db interface
type Store struct {
	client *pgxpool.Pool
	config *Config

	tracer       Tracer
	metrics      *metric.MeterProvider
	cfg          *config.Config
	log          *slog.Logger
	afterConnect AfterConnectFunc

	// opts are kept rather than applied at construction, so that Init can lay
	// down defaults and configuration first and let the options override them.
	opts []Option

	routing replica.Options
	router  *replica.Router

	// closed makes shutdown deterministic. Init starts a goroutine waiting on
	// context cancellation, and Close has to be able to retire it too — a
	// store that can only be shut down by canceling a context cannot be shut
	// down at all from a test, or from a service that rebuilds its stores.
	closeOnce sync.Once
	closed    chan struct{}
}
