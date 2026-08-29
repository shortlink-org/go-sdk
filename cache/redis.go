package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// RedisOption configures the client built by NewRedisClient.
type RedisOption func(*redisSettings)

// redisSettings holds what the options collected, before the client is built.
type redisSettings struct {
	tracer trace.TracerProvider
	meter  metric.MeterProvider
}

// WithRedisTracer turns on redisotel tracing, so that every command opens a
// client span. A nil provider is the same as not passing the option.
func WithRedisTracer(tracer trace.TracerProvider) RedisOption {
	return func(s *redisSettings) {
		s.tracer = tracer
	}
}

// WithRedisMeter turns on redisotel metrics (command duration, pool usage).
// A nil provider is the same as not passing the option.
func WithRedisMeter(meter metric.MeterProvider) RedisOption {
	return func(s *redisSettings) {
		s.meter = meter
	}
}

// RedisClient wraps the Redis client with additional error handling.
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client wrapper.
//
// Instrumentation is opt-in: without WithRedisTracer or WithRedisMeter the
// client is left as go-redis built it, and a service that exports nothing does
// not pay for the hooks.
//
//	client, err := cache.NewRedisClient(opts, cache.WithRedisTracer(tracer))
func NewRedisClient(opts *redis.Options, options ...RedisOption) (*RedisClient, error) {
	settings := &redisSettings{}
	for _, option := range options {
		option(settings)
	}

	client := redis.NewClient(opts)

	if settings.tracer != nil {
		err := redisotel.InstrumentTracing(client, redisotel.WithTracerProvider(settings.tracer))
		if err != nil {
			return nil, NewCacheError("instrument tracing", err)
		}
	}

	if settings.meter != nil {
		err := redisotel.InstrumentMetrics(client, redisotel.WithMeterProvider(settings.meter))
		if err != nil {
			return nil, NewCacheError("instrument metrics", err)
		}
	}

	return &RedisClient{client: client}, nil
}

// Set stores a key-value pair with optional expiration.
func (r *RedisClient) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	err := r.client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		return NewCacheError("set", err)
	}

	return nil
}

// Get retrieves a value by key.
func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}

	if err != nil {
		return "", NewCacheError("get", err)
	}

	return val, nil
}

// Delete removes a key.
func (r *RedisClient) Delete(ctx context.Context, key string) error {
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return NewCacheError("delete", err)
	}

	return nil
}

// Exists checks if a key exists.
func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, NewCacheError("exists", err)
	}

	return n > 0, nil
}

// Close closes the Redis connection.
func (r *RedisClient) Close() error {
	err := r.client.Close()
	if err != nil {
		return NewCacheError("close", err)
	}

	return nil
}

// Pipeline creates a new pipeline for batch operations
//
//nolint:ireturn // it's correct to return the interface
func (r *RedisClient) Pipeline() redis.Pipeliner {
	return r.client.Pipeline()
}

// Client returns the underlying Redis client.
func (r *RedisClient) Client() *redis.Client {
	return r.client
}
