package redis

import (
	"context"

	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidisotel"
	"github.com/shortlink-org/go-sdk/config"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
)

func New(tracer trace.TracerProvider, metrics *metric.MeterProvider, cfg *config.Config) *Store {
	return &Store{
		tracer:  tracer,
		metrics: metrics,
		cfg:     cfg,
	}
}

// Init - initialize
func (s *Store) Init(ctx context.Context) error {
	var err error

	// Set configuration
	s.setConfig()

	if len(s.config.Host) == 0 {
		return &StoreError{
			Op:      "init",
			Err:     ErrInvalidURI,
			Details: "redis host configuration is empty",
		}
	}

	// Connect to Redis
	s.client, err = rueidisotel.NewClient(rueidis.ClientOption{
		InitAddress: s.config.Host,
		Username:    s.config.Username,
		Password:    s.config.Password,
		SelectDB:    0, // use default DB
	}, rueidisotel.WithTracerProvider(s.tracer), rueidisotel.WithMeterProvider(s.metrics))
	if err != nil {
		return &StoreError{
			Op:      "init",
			Err:     ErrClientConnection,
			Details: err.Error(),
		}
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		s.client.Close()
	}()

	return nil
}

// GetConn - get connect
func (s *Store) GetConn() any {
	return s.client
}

// setConfig - set configuration
func (s *Store) setConfig() {
	s.cfg.SetDefault("STORE_REDIS_URI", "localhost:6379") // Redis Hosts
	s.cfg.SetDefault("STORE_REDIS_USERNAME", "")          // Redis Username
	s.cfg.SetDefault("STORE_REDIS_PASSWORD", "")          // Redis Password

	s.config = Config{
		Host:     s.cfg.GetStringSlice("STORE_REDIS_URI"),
		Username: s.cfg.GetString("STORE_REDIS_USERNAME"),
		Password: s.cfg.GetString("STORE_REDIS_PASSWORD"),
	}
}
