package mysql

import (
	"context"
	"net/url"

	"github.com/XSAM/otelsql"
	_ "github.com/go-sql-driver/mysql"
	"go.opentelemetry.io/otel/sdk/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
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
	// Set configuration
	err := s.setConfig()
	if err != nil {
		return &StoreError{
			Op:      "setConfig",
			Err:     err,
			Details: "failed to set mysql configuration",
		}
	}

	options := []otelsql.Option{
		otelsql.WithTracerProvider(s.tracer),
		otelsql.WithMeterProvider(s.metrics),
		otelsql.WithSQLCommenter(true),
	}

	// Connect to MySQL
	conn, connErr := otelsql.Open("mysql", s.config.URI, options...)
	if connErr != nil {
		return &StoreError{
			Op:      "init",
			Err:     ErrClientConnection,
			Details: connErr.Error(),
		}
	}

	s.client = conn

	// Check connection
	errPing := s.client.PingContext(ctx)
	if errPing != nil {
		s.client.Close() //nolint:errcheck,gosec // best-effort cleanup on ping failure

		return &PingConnectionError{
			Err: errPing,
		}
	}

	// Register DB stats to meter
	if _, err = otelsql.RegisterDBStatsMetrics(s.client, otelsql.WithAttributes(
		semconv.DBSystemNameMySQL,
	)); err != nil {
		return &StoreError{
			Op:      "init",
			Err:     err,
			Details: "failed to register DB stats metrics",
		}
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		errClose := s.close()
		if errClose != nil {
			// We can't return the error here since we're in a goroutine,
			// but in a real application you might want to log this
			_ = errClose
		}
	}()

	return nil
}

// GetConn - get connect
func (s *Store) GetConn() any {
	return s.client
}

// Close - close
func (s *Store) close() error {
	err := s.client.Close()
	if err != nil {
		return &StoreError{
			Op:      "close",
			Err:     err,
			Details: "failed to close mysql connection",
		}
	}

	return nil
}

// setConfig - set configuration
func (s *Store) setConfig() error {
	s.cfg.SetDefault("STORE_MYSQL_URI", "shortlink:shortlink@(localhost:3306)/shortlink") // MySQL URI

	// parse uri
	uri, err := url.Parse(s.cfg.GetString("STORE_MYSQL_URI"))
	if err != nil {
		return &StoreError{
			Op:      "setConfig",
			Err:     ErrInvalidDSN,
			Details: "parsing MySQL URI from environment variable",
		}
	}

	values := uri.Query()
	values.Add("parseTime", "true")

	uri.RawQuery = values.Encode()

	s.config = Config{
		URI: uri.String(),
	}

	return nil
}
