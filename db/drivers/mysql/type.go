package mysql

import (
	"database/sql"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - configuration
type Config struct {
	URI string
}

// Store implementation of db interface
type Store struct {
	client *sql.DB

	tracer  trace.TracerProvider
	metrics *metric.MeterProvider

	config Config
	cfg    *config.Config
}
