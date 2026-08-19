package dgraph

import (
	"context"
	"fmt"

	"github.com/dgraph-io/dgo/v250"
	"github.com/dgraph-io/dgo/v250/protos/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

// Config - config
type Config struct {
	URL string
}

// Store - store struct
type Store struct {
	log logger.Logger
	// conn is held because dgo.Dgraph.Close cannot be relied on: as of
	// v250.0.0 NewRoundRobinClient fills a local conns slice and then builds
	// the client without it, so Close iterates nothing and every connection
	// outlives the process's shutdown. Owning the connection here is also what
	// lets Init release it when the migration fails.
	conn   *grpc.ClientConn
	client *dgo.Dgraph
	config Config
	cfg    *config.Config
}

func New(log logger.Logger, cfg *config.Config) *Store {
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

	conn, err := grpc.NewClient(
		s.config.URL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
	)
	if err != nil {
		return &StoreError{
			Driver:  driverName,
			Op:      "connect",
			Err:     fmt.Errorf("%w: %w", ErrDgraphClient, err),
			Details: "failed to create the gRPC connection",
		}
	}

	s.conn = conn
	// The replacements dgo suggests, NewClient and Open, construct the client
	// without recording the connections they opened, so their Close is a no-op
	// and the connection outlives shutdown. Until that is fixed upstream, the
	// deprecated constructor is the only one that leaves the connection ours
	// to close.
	//nolint:staticcheck // dgo v250.0.0: NewClient/Open leak their connections
	s.client = dgo.NewDgraphClient(api.NewDgraphClient(conn))

	errMigrate := s.migrate(ctx)
	if errMigrate != nil {
		// The connection is ours until Init succeeds. Returning without
		// closing it leaks the connection and its resolver goroutine, and a
		// caller that retries Init leaks one per attempt.
		s.close(ctx)

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

		s.close(ctx)
	}()

	return nil
}

// close releases the connection. Safe to call on a half-built Store.
func (s *Store) close(ctx context.Context) {
	if s.conn == nil {
		return
	}

	errClose := s.conn.Close()
	if errClose != nil {
		s.log.ErrorWithContext(ctx, errClose.Error())
	}

	s.conn = nil
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
			s.log.ErrorWithContext(ctx, errDiscard.Error())
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
	s.cfg.SetDefault("STORE_DGRAPH_URI", "localhost:9080") // DGRAPH URI

	s.config = Config{
		URL: s.cfg.GetString("STORE_DGRAPH_URI"),
	}
}
