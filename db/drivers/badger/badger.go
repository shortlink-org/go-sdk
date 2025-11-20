package badger

import (
	"context"
	"fmt"

	"github.com/dgraph-io/badger/v4"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - config
type Config struct {
	Path string
}

// Store implementation of db interface
type Store struct {
	client *badger.DB
	config Config
	cfg    *config.Config
}

// New creates a Badger store configured via cfg.
func New(cfg *config.Config) *Store {
	return &Store{
		config: Config{
			Path: "",
		},
		cfg: cfg,
	}
}

// Init - initialize
func (s *Store) Init(ctx context.Context) error {
	var err error

	// Set configuration
	s.setConfig()

	s.client, err = badger.Open(badger.DefaultOptions(s.config.Path))
	if err != nil {
		return &StoreError{
			Op:      "init",
			Err:     fmt.Errorf("%w: %w", ErrBadgerOpen, err),
			Details: "opening Badger DB at path " + s.config.Path,
		}
	}

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

// Close - close
func (s *Store) close() error {
	err := s.client.Close()
	if err != nil {
		return &StoreError{
			Op:      "close",
			Err:     fmt.Errorf("%w: %w", ErrBadgerClose, err),
			Details: "closing Badger DB",
		}
	}

	return nil
}

// setConfig - set configuration
func (s *Store) setConfig() {
	s.cfg.SetDefault("STORE_BADGER_PATH", "/tmp/links.badger") // Badger path to file

	s.config = Config{
		Path: s.cfg.GetString("STORE_BADGER_PATH"),
	}
}
