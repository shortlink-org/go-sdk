package clickhouse

import (
	"context"
	"fmt"

	"github.com/uptrace/go-clickhouse/ch"
	"github.com/uptrace/go-clickhouse/chdebug"
	"github.com/uptrace/go-clickhouse/chotel"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - config
type Config struct {
	URI string
}

// Store implementation of db interface
type Store struct {
	client *ch.DB
	config Config
	cfg    *config.Config
}

// New creates a ClickHouse store configured via cfg.
func New(cfg *config.Config) *Store {
	return &Store{
		config: Config{
			URI: "",
		},
		cfg: cfg,
	}
}

// Init - initialize
func (s *Store) Init(ctx context.Context) error {
	// Set configuration
	s.setConfig()

	// Connect to Clickhouse
	clickhouseDB := ch.Connect(ch.WithDSN(s.config.URI))
	clickhouseDB.AddQueryHook(chdebug.NewQueryHook(chdebug.WithVerbose(true)))
	clickhouseDB.AddQueryHook(chotel.NewQueryHook())

	err := clickhouseDB.Ping(ctx)
	if err != nil {
		return &StoreError{
			Op:      "init",
			Err:     fmt.Errorf("%w: %w", ErrPing, err),
			Details: "pinging Clickhouse after connection",
		}
	}

	s.client = clickhouseDB

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		s.close() //nolint:errcheck,gosec // background cleanup
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
			Err:     fmt.Errorf("%w: %w", ErrClose, err),
			Details: "closing Clickhouse connection",
		}
	}

	return nil
}

// setConfig - set configuration
func (s *Store) setConfig() {
	s.cfg.SetDefault("STORE_CLICKHOUSE_URI", "clickhouse://localhost:9000/default?sslmode=disable") // Clickhouse URI

	s.config = Config{
		URI: s.cfg.GetString("STORE_CLICKHOUSE_URI"),
	}
}
