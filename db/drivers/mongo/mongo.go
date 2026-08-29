package mongo

import (
	"context"

	_ "github.com/golang-migrate/migrate/v4/database/mongodb"
	_ "github.com/johejo/golang-migrate-extra/source/file"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
	storeOptions "github.com/shortlink-org/go-sdk/db/options"
)

// New creates a MongoDB store configured via cfg.
func New(
	tracer trace.TracerProvider,
	metrics *metric.MeterProvider,
	cfg *config.Config,
	opts ...Option,
) *Store {
	s := &Store{cfg: cfg, tracer: tracer, metrics: metrics}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Init - initialize
func (s *Store) Init(ctx context.Context) error {
	var err error

	// Set configuration
	s.setConfig()

	// Connect to MongoDB
	opts := options.Client().
		ApplyURI(s.config.URI).
		SetCompressors([]string{"snappy", "zlib", "zstd"}).
		SetAppName(s.cfg.GetString("SERVICE_NAME")).
		SetMonitor(s.monitor()).
		SetRetryReads(true).
		SetRetryWrites(true)

	// Driver options are applied last, so that they can override the defaults above.
	for _, fn := range s.clientOptions {
		fn(opts)
	}

	s.client, err = mongo.Connect(opts)
	if err != nil {
		return &StoreError{
			Driver:  driverName,
			Op:      "init",
			Err:     ErrClientConnection,
			Details: err.Error(),
		}
	}

	// Check connecting
	err = s.client.Ping(ctx, readpref.Primary())
	if err != nil {
		return &PingConnectionError{
			Driver: driverName,
			Err:    err,
		}
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		errClose := s.close(ctx)
		if errClose != nil {
			// We can't return the error here since we're in a goroutine,
			// but in a real application you might want to log this
			_ = errClose
		}
	}()

	return nil
}

// monitor builds the command monitor the client reports through, or nil when
// there is nothing to report to. The providers are passed only when they exist:
// otelmongo calls Tracer and Meter on whatever it is given, and a typed nil
// provider panics there rather than degrading to a no-op.
func (s *Store) monitor() *event.CommandMonitor {
	if s.tracer == nil && s.metrics == nil {
		return nil
	}

	opts := []otelmongo.Option{}
	if s.tracer != nil {
		opts = append(opts, otelmongo.WithTracerProvider(s.tracer))
	}

	if s.metrics != nil {
		opts = append(opts, otelmongo.WithMeterProvider(s.metrics))
	}

	return otelmongo.NewMonitor(opts...)
}

// GetConn - get connect
func (s *Store) GetConn() any {
	return s.client
}

// Close - close
func (s *Store) close(ctx context.Context) error {
	err := s.client.Disconnect(ctx)
	if err != nil {
		return &StoreError{
			Driver:  driverName,
			Op:      "close",
			Err:     err,
			Details: "failed to disconnect mongodb client",
		}
	}

	return nil
}

// setConfig - set configuration
func (s *Store) setConfig() {
	s.cfg.SetDefault("STORE_MONGODB_URI", "mongodb://shortlink:password@localhost:27017/shortlink") // MongoDB URI
	s.cfg.SetDefault("STORE_MODE_WRITE", storeOptions.MODE_SINGLE_WRITE)                            // mode write to db

	s.config = Config{
		URI:  s.cfg.GetString("STORE_MONGODB_URI"),
		mode: s.cfg.GetInt("STORE_MODE_WRITE"),
	}
}
