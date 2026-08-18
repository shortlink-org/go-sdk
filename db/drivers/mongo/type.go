package mongo

import (
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - config
type Config struct {
	URI  string
	mode int
}

// ClientOptionsFunc customizes the driver options right before connecting.
type ClientOptionsFunc func(*options.ClientOptions)

// Option is a functional option for Store configuration.
type Option func(*Store)

// WithClientOptions registers a callback that receives the client options once the
// defaults have been applied and before Connect, so it can add to them or override
// them. It is the seam for everything that cannot be spelled as a config string: a
// custom TLS config, pool tuning, a server API version, auto encryption.
//
//	store, err := db.New(ctx, log, tracer, metrics, cfg,
//		mongo.With(mongo.WithClientOptions(func(opts *options.ClientOptions) {
//			opts.SetAutoEncryptionOptions(autoEncryption)
//		})),
//	)
//
// Callbacks run in the order they were given. Note that auto encryption also
// requires building with `-tags cse` and linking libmongocrypt: without them the
// driver compiles its stubs and the encryption calls fail at runtime.
func WithClientOptions(fn ClientOptionsFunc) Option {
	return func(s *Store) {
		s.clientOptions = append(s.clientOptions, fn)
	}
}

// Store implementation of db interface
type Store struct {
	client *mongo.Client
	config Config
	cfg    *config.Config

	clientOptions []ClientOptionsFunc
}
