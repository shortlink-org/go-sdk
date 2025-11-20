package leveldb

import (
	"context"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - config
type Config struct {
	Path string
}

// Store implementation of db interface
type Store struct {
	client *leveldb.DB
	config Config
	cfg    *config.Config
}

// New creates a LevelDB store configured via cfg.
func New(cfg *config.Config) *Store {
	return &Store{
		config: Config{},
		cfg:    cfg,
	}
}

// Init - initialize
func (s *Store) Init(ctx context.Context) error {
	var err error

	// Set configuration
	s.setConfig()

	s.client, err = leveldb.OpenFile(s.config.Path, nil)
	if err != nil {
		return &StoreError{
			Op:      "init",
			Err:     ErrDatabaseOpen,
			Details: err.Error(),
		}
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		s.close() //nolint:errcheck,gosec // best effort shutdown in tests
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
			Details: "failed to close leveldb database",
		}
	}

	return nil
}

// setConfig - set configuration
func (s *Store) setConfig() {
	s.cfg.SetDefault("STORE_LEVELDB_PATH", "/tmp/links.db") // LevelDB path to file

	s.config = Config{
		Path: s.cfg.GetString("STORE_LEVELDB_PATH"),
	}
}
