package neo4j

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - configuration
type Config struct {
	URI      string
	login    string
	password string
}

// Store implementation of db interface
type Store struct {
	client neo4j.Driver
	config Config
	cfg    *config.Config
}

// New creates a Neo4j store configured via cfg.
func New(cfg *config.Config) *Store {
	return &Store{cfg: cfg}
}

// Init - init connection
func (s *Store) Init(ctx context.Context) error {
	// Set configuration
	err := s.setConfig()
	if err != nil {
		return &StoreError{
			Driver:  driverName,
			Op:      "setConfig",
			Err:     err,
			Details: "failed to set neo4j configuration",
		}
	}

	client, err := neo4j.NewDriver(s.config.URI, neo4j.BasicAuth(s.config.login, s.config.password, ""))
	if err != nil {
		return &StoreError{
			Driver:  driverName,
			Op:      "init",
			Err:     ErrClientConnection,
			Details: err.Error(),
		}
	}

	// NewDriver validates the URI but connects lazily. Verify connectivity here
	// so Init cannot report success for an unreachable server or bad credentials.
	err = client.VerifyConnectivity(ctx)
	if err != nil {
		// The client is ours until Init succeeds. Without this cleanup, each
		// failed initialization leaves the driver's connection pool behind.
		err = errors.Join(err, client.Close(context.WithoutCancel(ctx)))

		return &PingConnectionError{
			Driver: driverName,
			Err:    err,
		}
	}

	s.client = client

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		err := s.close(context.WithoutCancel(ctx))
		if err != nil {
			// We can't return the error here since we're in a goroutine,
			// but in a real application you might want to log this
			_ = err
		}
	}()

	return nil
}

// GetConn - return connection
func (s *Store) GetConn() any {
	return s.client
}

// Close - close connection
func (s *Store) close(ctx context.Context) error {
	err := s.client.Close(ctx)
	if err != nil {
		return &StoreError{
			Driver:  driverName,
			Op:      "close",
			Err:     err,
			Details: "failed to close neo4j connection",
		}
	}

	return nil
}

// setConfig - set configuration
func (s *Store) setConfig() error {
	// Neo4j 4.0, defaults to no TLS therefore use bolt:// or neo4j://
	// Neo4j 3.5, defaults to self-signed certificates, TLS on, therefore use bolt+ssc:// or neo4j+ssc://
	s.cfg.SetDefault("STORE_NEO4J_URI", "neo4j://localhost:7687") // NEO4J URI

	uri := s.cfg.GetString("STORE_NEO4J_URI")

	params, err := url.ParseRequestURI(uri)
	if err != nil {
		return &StoreError{
			Driver:  driverName,
			Op:      "setConfig",
			Err:     err,
			Details: "invalid neo4j URI",
		}
	}

	password, _ := params.User.Password()

	s.config = Config{
		URI:      fmt.Sprintf("%s://%s", params.Scheme, params.Host),
		login:    params.User.Username(),
		password: password,
	}

	return nil
}
