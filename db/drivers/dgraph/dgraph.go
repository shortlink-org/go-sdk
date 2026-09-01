package dgraph

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dgraph-io/dgo/v250"
	"github.com/dgraph-io/dgo/v250/protos/api"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - config
type Config struct {
	URL string
}

// Store - store struct
type Store struct {
	log    *slog.Logger
	client *dgo.Dgraph
	config Config
	cfg    *config.Config
}

func New(log *slog.Logger, cfg *config.Config) *Store {
	return &Store{
		log: log,
		config: Config{
			URL: "",
		},
		cfg: cfg,
	}
}

// Init - initialize
func (s *Store) Init(ctx context.Context) error {
	// Set configuration
	s.setConfig()

	// dgo parses the connection string itself, which is what carries the ACL
	// credentials, the TLS mode and the namespace. It pings the server before
	// returning, so a failure here means unreachable or misspelled, not merely
	// unconfigured.
	client, err := dgo.Open(s.config.URL)
	if err != nil {
		return &StoreError{
			Driver:  driverName,
			Op:      "connect",
			Err:     fmt.Errorf("%w: %w", ErrDgraphClient, err),
			Details: "failed to create the Dgraph client",
		}
	}

	s.client = client

	errMigrate := s.migrate(ctx)
	if errMigrate != nil {
		// The client is ours until Init succeeds, so it is released here
		// rather than left to the caller, who has no handle on it.
		s.close()

		return &StoreError{
			Driver:  driverName,
			Op:      "migrate",
			Err:     fmt.Errorf("%w: %w", ErrDgraphMigrate, errMigrate),
			Details: "failed to alter the schema",
		}
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		s.close()
	}()

	return nil
}

// close releases the client. Safe to call on a half-built Store.
//
// As of dgo v250.0.0 this frees less than it appears to: NewRoundRobinClient,
// which every constructor goes through, fills a local conns slice and then
// builds the client without it, so Dgraph.Close ranges over nothing and the
// gRPC connections stay open. The connections are not reachable through any
// exported method, so there is nothing this driver can do about it beyond
// calling Close and expecting it to start working.
func (s *Store) close() {
	if s.client == nil {
		return
	}

	s.client.Close()
	s.client = nil
}

// GetConn - get connect
func (s *Store) GetConn() any {
	return s.client
}

// Migrate - init structure
func (s *Store) migrate(ctx context.Context) error {
	txn := s.client.NewTxn()

	defer func() {
		errDiscard := txn.Discard(ctx)
		if errDiscard != nil {
			s.log.ErrorContext(ctx, errDiscard.Error())
		}
	}()

	// TODO: use migration tool
	operation := new(api.Operation)

	operation.Schema = `
type Link {
	url: string
	hash: string
	describe: string
	created_at: datetime
	updated_at: datetime
}

url: string @index(term) @lang .
hash: string @index(term) @lang .
	describe: string @index(term) @lang .
	created_at: datetime .
	updated_at: datetime .
`

	// Wrapped by the caller, which names the phase — returning a StoreError
	// here too would repeat the same details inside itself.
	return s.client.Alter(ctx, operation)
}

// setConfig - set configuration
func (s *Store) setConfig() {
	s.cfg.SetDefault("STORE_DGRAPH_URI", "dgraph://localhost:9080?sslmode=disable") // DGRAPH URI

	s.config = Config{
		URL: s.cfg.GetString("STORE_DGRAPH_URI"),
	}
}
