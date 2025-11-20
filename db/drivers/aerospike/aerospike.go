package aerospike

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	aero "github.com/aerospike/aerospike-client-go"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - config
type Config struct {
	host string
	port int
}

// Store implementation of db interface
type Store struct {
	client *aero.Client
	config Config
	cfg    *config.Config
}

// New creates an Aerospike store configured via cfg.
func New(cfg *config.Config) *Store {
	return &Store{
		config: Config{
			host: "",
			port: 0,
		},
		cfg: cfg,
	}
}

// Init - initialize
func (s *Store) Init(ctx context.Context) error {
	// Set configuration
	err := s.setConfig()
	if err != nil {
		return &StoreError{
			Op:      "init",
			Err:     err,
			Details: "failed to set configuration",
		}
	}

	// Connect to Aerospike
	client, clientErr := aero.NewClient(s.config.host, s.config.port)
	if clientErr != nil {
		return &StoreError{
			Op:      "init",
			Err:     fmt.Errorf("%w: %w", ErrClientConnection, clientErr),
			Details: fmt.Sprintf("unable to connect to Aerospike at %s:%d", s.config.host, s.config.port),
		}
	}

	s.client = client

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
func (s *Store) setConfig() error {
	s.cfg.SetDefault("STORE_AEROSPIKE_URI", "tcp://localhost:3000") // Aerospike URI

	conf, err := url.Parse(s.cfg.GetString("STORE_AEROSPIKE_URI"))
	if err != nil {
		return &StoreError{
			Op:      "setConfig",
			Err:     fmt.Errorf("%w: %w", ErrInvalidURI, err),
			Details: "parsing Aerospike URI from environment variable",
		}
	}

	port, err := strconv.Atoi(conf.Port())
	if err != nil {
		return &StoreError{
			Op:      "setConfig",
			Err:     fmt.Errorf("%w: %w", ErrInvalidPort, err),
			Details: "parsing port from URI: " + conf.Port(),
		}
	}

	s.config = Config{
		host: conf.Hostname(),
		port: port,
	}

	return nil
}
