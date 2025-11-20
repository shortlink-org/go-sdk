package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shortlink-org/go-sdk/config"
	"go.opentelemetry.io/otel/sdk/metric"
)

// Config - config
type Config struct {
	mode   int
	config *pgxpool.Config
}

// Store implementation of db interface
type Store struct {
	client *pgxpool.Pool
	config *Config

	tracer  Tracer
	metrics *metric.MeterProvider
	cfg     *config.Config
}
