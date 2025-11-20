package edgedb

import (
	"context"
	"fmt"

	"github.com/geldata/gel-go"
	"github.com/geldata/gel-go/gelcfg"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - config
type Config struct {
	URI string
}

// Store implementation of db interface
type Store struct {
	client *gel.Client
	config Config
	cfg    *config.Config
}

// New creates a Store instance.
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

	// Connect to EdgeDB
	options := gelcfg.Options{
		Branch: "shortlink",
	}

	client, err := gel.CreateClientDSN(s.config.URI, options)
	if err != nil {
		return &StoreError{
			Op:      "CreateClientDSN",
			Err:     fmt.Errorf("%w: %w", ErrConnect, err),
			Details: "failed to connect to EdgeDB at " + s.config.URI,
		}
	}

	s.client = client

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		s.close() //nolint:errcheck,gosec // background cleanup, best effort in tests
	}()

	return nil
}

// GetConn - get connect
func (s *Store) GetConn() any {
	return s.client
}

// close - close
func (s *Store) close() error {
	err := s.client.Close()
	if err != nil {
		return &StoreError{
			Op:      "close",
			Err:     fmt.Errorf("%w: %w", ErrClose, err),
			Details: "failed to close EdgeDB connection",
		}
	}

	return nil
}

// setConfig - set configuration
func (s *Store) setConfig() {
	s.cfg.SetDefault("STORE_EDGEDB_URI", "edgedb://localhost:5656") // EdgeDB URI

	s.config = Config{
		URI: s.cfg.GetString("STORE_EDGEDB_URI"),
	}
}
