package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shortlink-org/go-sdk/config"
	"go.opentelemetry.io/otel/sdk/metric"
)

// AfterConnectFunc is a callback executed after each new connection is established.
type AfterConnectFunc func(ctx context.Context, conn *pgx.Conn) error

// Config - config
type Config struct {
	mode   int
	config *pgxpool.Config
}

// Option is a functional option for Store configuration.
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
	afterConnect AfterConnectFunc
}
