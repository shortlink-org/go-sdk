package redis

import (
	"context"

	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidisotel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
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
			Driver:  driverName,
			Op:      "init",
			Err:     ErrInvalidURI,
			Details: "redis host configuration is empty",
		}
	}

	// Connect to Redis.
	//
	// The instrumentation options are only passed when a provider exists:
	// rueidisotel calls Meter and Tracer on whatever it is given, and a typed
	// nil provider panics there rather than degrading to a no-op.
	options := []rueidisotel.Option{}
	if s.tracer != nil {
		options = append(options, rueidisotel.WithTracerProvider(s.tracer))
	}

	if s.metrics != nil {
		options = append(options, rueidisotel.WithMeterProvider(s.metrics))
	}

	s.client, err = rueidisotel.NewClient(rueidis.ClientOption{
		InitAddress: s.config.Host,
		Username:    s.config.Username,
		Password:    s.config.Password,
		SelectDB:    0, // use default DB
	}, options...)
	if err != nil {
		return &StoreError{
			Driver:  driverName,
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
