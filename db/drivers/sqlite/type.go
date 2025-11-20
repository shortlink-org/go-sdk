package sqlite

import (
	"database/sql"

	"github.com/shortlink-org/go-sdk/config"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
)

// Config - config
type Config struct {
	Path string
}

// Store implementation of db interface
type Store struct {
	client *sql.DB
	config Config

	tracer  trace.TracerProvider
	metrics *metric.MeterProvider
	cfg     *config.Config
}
