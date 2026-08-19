package dgraph

import (
	"context"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgo/v250"
	"github.com/dgraph-io/dgo/v250/protos/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

// connSchemePrefix marks a value that is a full connection string rather than
// a bare host:port.
const connSchemePrefix = "dgraph://"

// Config - config
type Config struct {
	URL string
}

// Store - store struct
type Store struct {
	log    logger.Logger
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

	client, err := s.connect()
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

// connect builds the client from the configured URI.
//
// Two forms are accepted. A bare host:port is what this driver has always
// taken and stays the default. A dgraph:// connection string is the form dgo
// parses itself, and the only way to reach a server behind ACL credentials,
// TLS, or a non-default namespace without adding an option per knob:
//
//	dgraph://user:pass@host:9080?sslmode=verify-ca&namespace=1
//
// Either way the client pings the server before returning, so a failure here
// means unreachable rather than merely misconfigured.
func (s *Store) connect() (*dgo.Dgraph, error) {
	if strings.HasPrefix(s.config.URL, connSchemePrefix) {
		return dgo.Open(s.config.URL)
	}

	// Plaintext, as this driver has always been. dgo sets no transport
	// credentials of its own and fails rather than guessing, so the choice has
	// to be made here; TLS is reached through the connection-string form.
	return dgo.NewClient(s.config.URL,
		dgo.WithGrpcOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		dgo.WithGrpcOption(grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name))),
	)
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
