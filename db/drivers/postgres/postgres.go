package postgres

import (
	"context"
	"fmt"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq" // need for init PostgreSQL interface
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db/options"
)

// New return new instance of Store
func New(tracer trace.TracerProvider, metrics *metric.MeterProvider, cfg *config.Config, opts ...Option) *Store {
	return &Store{
		tracer: Tracer{
			TracerProvider: tracer,
		},
		metrics: metrics,
		cfg:     cfg,
		opts:    opts,
	}
}

// Init - initialize
func (s *Store) Init(ctx context.Context) error {
	var err error

	// Resolve options: defaults, then configuration, then the functional
	// options, so that an option always wins over an environment variable.
	s.routing = routingFromConfig(s.cfg)

	for _, opt := range s.opts {
		opt(s)
	}

	// Set configuration
	s.config, err = getConfig(&s.tracer, s.cfg)
	if err != nil {
		return &StoreError{
			Op:      opInit,
			Err:     err,
			Details: "failed to get postgres connection config",
		}
	}

	// Apply AfterConnect if provided
	if s.afterConnect != nil {
		s.config.config.AfterConnect = s.afterConnect
	}

	// Connect to Postgres
	s.client, err = pgxpool.NewWithConfig(ctx, s.config.config)
	if err != nil {
		return &StoreError{
			Op:      opInit,
			Err:     err,
			Details: "failed to open the database",
		}
	}

	// Check connecting
	err = s.client.Ping(ctx)
	if err != nil {
		s.client.Close()

		return &PingConnectionError{err}
	}

	// Build the read-replica router. Without replica DSNs this is a no-op and
	// the store behaves exactly as it did before routing existed.
	s.router, err = s.buildRouter(ctx)
	if err != nil {
		s.client.Close()

		return err
	}

	// Graceful shutdown, either by canceling ctx or by calling Close.
	s.closed = make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			s.Close()
		case <-s.closed:
		}
	}()

	return nil
}

// Close releases the connection pools and stops the replica poller. It is
// idempotent, it waits for the poller goroutine to exit, and it is what
// context cancellation ends up calling.
//
// Waiting matters: a poller that outlives its store shows up as a leaked
// goroutine in tests and as a slow climb in a service that rebuilds stores.
func (s *Store) Close() {
	s.closeOnce.Do(func() {
		if s.closed != nil {
			close(s.closed)
		}

		if s.router != nil {
			// Closes the replica pools and joins the poller goroutine.
			s.router.Close()
		}

		if s.client != nil {
			s.client.Close()
		}
	})
}

// GetConn - get connect
//
// It returns the primary pool, and always will: db.Conn[*pgxpool.Pool] is
// public API, and downstream consumers of this SDK are invisible from here.
// The read-replica router is reached through RouterFrom.
func (s *Store) GetConn() any {
	return s.client
}

// setConfig - set configuration
func getConfig(tracer *Tracer, cfg *config.Config) (*Config, error) {
	dbinfo := fmt.Sprintf("postgres://%s:%s@localhost:5432/%s?sslmode=disable", "postgres", "shortlink", "shortlink")

	cfg.SetDefault("STORE_POSTGRES_URI", dbinfo)                  // Postgres URI
	cfg.SetDefault("STORE_MODE_WRITE", options.MODE_SINGLE_WRITE) // mode write to db

	// Create pool config
	cnfPool, err := pgxpool.ParseConfig(cfg.GetString("STORE_POSTGRES_URI"))
	if err != nil {
		return nil, &StoreError{
			Op:      "ParseConfig",
			Err:     err,
			Details: "failed to parse postgres connection config",
		}
	}

	instrument(cnfPool, tracer)

	return &Config{
		config: cnfPool,
		mode:   cfg.GetInt("STORE_MODE_WRITE"),
	}, nil
}

// instrument wires OpenTelemetry into a pool config. Replica pools get the
// same treatment as the primary, so a query is traced identically wherever it
// runs.
func instrument(pool *pgxpool.Config, tracer *Tracer) {
	params := []otelpgx.Option{
		otelpgx.WithIncludeQueryParameters(),
	}

	if tracer.TracerProvider != nil {
		params = append(params, otelpgx.WithTracerProvider(tracer))
	}

	pool.ConnConfig.Tracer = otelpgx.NewTracer(params...)
}
