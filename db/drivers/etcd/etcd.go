package etcd

import (
	"context"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - config
type Config struct {
	URI         []string
	DialTimeout time.Duration
}

// Store implementation of db interface
type Store struct {
	client *clientv3.Client
	config Config
	cfg    *config.Config
}

// New creates an etcd store configured via cfg.
func New(cfg *config.Config) *Store {
	return &Store{cfg: cfg}
}

// Init - initialize
func (s *Store) Init(ctx context.Context) error {
	// Set configuration
	s.setConfig()

	// Connect to ETCD
	var err error

	s.client, err = clientv3.New(clientv3.Config{
		Endpoints:   s.config.URI,
		DialTimeout: s.config.DialTimeout,
	})
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

		s.close() //nolint:errcheck // background cleanup
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
			Err:     err,
			Details: "failed to close etcd client",
		}
	}

	return nil
}

// setConfig - set configuration
func (s *Store) setConfig() {
	s.cfg.SetDefault("STORE_ETCD_URI", "localhost:2379") // ETCD URI
	s.cfg.SetDefault("STORE_ETCD_TIMEOUT", "5s")         // ETCD timeout

	etcd := strings.Split(s.cfg.GetString("STORE_ETCD_URI"), ",")

	s.config = Config{
		URI:         etcd,
		DialTimeout: s.cfg.GetDuration("STORE_ETCD_TIMEOUT"),
	}
}
