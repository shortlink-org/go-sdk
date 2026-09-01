package cache

import (
	"context"
	"time"

	"github.com/redis/rueidis"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db"
	"github.com/shortlink-org/go-sdk/db/drivers/redis"
)

// clientCacheTTLKey configures the client-side cache from the environment.
// It follows the STORE_REDIS_ scheme the driver already uses; zero, the
// default, leaves the local layer off.
const clientCacheTTLKey = "STORE_REDIS_CLIENT_CACHE_TTL"

// Option configures the Redis adapter.
type Option func(*settings)

// settings holds what the options collected, before the client is opened.
type settings struct {
	tracer        trace.TracerProvider
	meter         *metric.MeterProvider
	clientSideTTL time.Duration
}

// WithTracer turns on rueidis tracing, so that every command opens a client
// span. A nil provider is the same as not passing the option: instrumentation
// is opt-in, and a service that exports nothing neither pays for the hooks nor
// has to depend on go-sdk/observability to start.
func WithTracer(tracer trace.TracerProvider) Option {
	return func(s *settings) {
		s.tracer = tracer
	}
}

// WithMeter turns on rueidis metrics. A nil provider is the same as not
// passing the option.
func WithMeter(meter *metric.MeterProvider) Option {
	return func(s *settings) {
		s.meter = meter
	}
}

// WithClientSideCache keeps a local copy of every value read, for at most ttl,
// through rueidis client-side caching. It is off by default; a ttl of zero or
// less turns it back off.
//
// The local layer is DoCache rather than an in-process LRU on purpose. Redis
// tracks which keys this connection has cached and pushes an invalidation when
// one of them changes, so a Delete on any replica drops the copy held by all
// of them. A TinyLFU or any other unsynchronised local cache cannot do that:
// nothing invalidates it, and every replica but the one that called Delete
// keeps serving the stale value until its own TTL runs out. For a catalog
// that is tolerable; for revoking a session it is a hole.
func WithClientSideCache(ttl time.Duration) Option {
	return func(s *settings) {
		s.clientSideTTL = ttl
	}
}

// Redis is the Cache backed by a single rueidis client.
type Redis struct {
	client rueidis.Client

	// clientSideTTL is the client-side cache window; zero means reads go to
	// the server every time.
	clientSideTTL time.Duration
}

// NewRedis opens the cache through the go-sdk/db redis driver, which reads
// STORE_REDIS_URI, STORE_REDIS_USERNAME and STORE_REDIS_PASSWORD.
//
// The returned Redis owns the connection: close it alongside the other
// connections the service opened.
func NewRedis(ctx context.Context, cfg *config.Config, opts ...Option) (*Redis, error) {
	cfg.SetDefault(clientCacheTTLKey, time.Duration(0))

	//nolint:exhaustruct // the zero value of every field is the "not configured" case
	options := &settings{clientSideTTL: cfg.GetDuration(clientCacheTTLKey)}
	for _, opt := range opts {
		opt(options)
	}

	store := redis.New(options.tracer, options.meter, cfg)

	err := store.Init(ctx)
	if err != nil {
		return nil, &InitCacheError{err: err}
	}

	conn, err := db.Conn[rueidis.Client](store)
	if err != nil {
		return nil, &InitCacheError{err: err}
	}

	return &Redis{
		client:        conn,
		clientSideTTL: max(options.clientSideTTL, 0),
	}, nil
}

// Get returns the stored bytes, or ErrMiss when the key is absent.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	// The builder is single-use, so the branch has to be taken before it is
	// finalized: Cache() and Build() each consume it.
	var result rueidis.RedisResult

	if r.clientSideTTL > 0 {
		result = r.client.DoCache(ctx, r.client.B().Get().Key(key).Cache(), r.clientSideTTL)
	} else {
		result = r.client.Do(ctx, r.client.B().Get().Key(key).Build())
	}

	value, err := result.AsBytes()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return nil, ErrMiss
		}

		return nil, NewCacheError("get", err)
	}

	return value, nil
}

// Set stores value under key for ttl. A ttl of zero or less stores nothing.
func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}

	cmd := r.client.B().Set().Key(key).Value(rueidis.BinaryString(value)).Px(ttl).Build()

	err := r.client.Do(ctx, cmd).Error()
	if err != nil {
		return NewCacheError("set", err)
	}

	return nil
}

// Delete removes the given keys; keys that are not there are ignored.
func (r *Redis) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	err := r.client.Do(ctx, r.client.B().Del().Key(keys...).Build()).Error()
	if err != nil {
		return NewCacheError("delete", err)
	}

	return nil
}

// Close releases the connection.
func (r *Redis) Close() error {
	r.client.Close()

	return nil
}
