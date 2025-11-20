package couchbase

import (
	"context"
	"fmt"

	"github.com/couchbase/gocb/v2"
)

// configProvider is the minimal subset of configuration behavior the driver relies on.
type configProvider interface {
	SetDefault(key string, value any)
	GetString(key string) string
}

type couchbaseConfig struct {
	uri     string
	options gocb.ClusterOptions
}

// Store implementation of db interface
type Store struct {
	client *gocb.Cluster
	config *couchbaseConfig
	cfg    configProvider
}

// New creates a Couchbase store configured via cfg.
func New(cfg configProvider) *Store {
	return &Store{
		config: nil,
		cfg:    cfg,
	}
}

// Init - initialize
func (s *Store) Init(ctx context.Context) error {
	var err error

	// Set configuration
	s.setConfig()

	s.client, err = gocb.Connect(s.config.uri, s.config.options)
	if err != nil {
		return &StoreError{
			Op:      "Connect",
			Err:     fmt.Errorf("%w: %w", ErrCouchbaseConnect, err),
			Details: "failed to connect to Couchbase cluster",
		}
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		//nolint:errcheck // ignore
		_ = s.client.Close(nil)
	}()

	return nil
}

// setConfig - set configuration
func (s *Store) setConfig() {
	defaultURI := "couchbase://localhost"

	if s.cfg != nil {
		s.cfg.SetDefault("STORE_COUCHBASE_URI", defaultURI)
	}

	uri := defaultURI

	if s.cfg != nil {
		if configuredURI := s.cfg.GetString("STORE_COUCHBASE_URI"); configuredURI != "" {
			uri = configuredURI
		}
	}

	var options gocb.ClusterOptions

	s.config = &couchbaseConfig{
		uri:     uri,
		options: options,
	}
}
