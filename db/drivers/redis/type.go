package redis

import (
	"github.com/redis/rueidis"
	"github.com/shortlink-org/go-sdk/config"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
)

// Config - config
type Config struct {
	Username string
	Password string
	Host     []string
}

// Store implementation of db interface
type Store struct {
	client rueidis.Client

	tracer  trace.TracerProvider
	metrics *metric.MeterProvider

	config Config
	cfg    *config.Config
}
