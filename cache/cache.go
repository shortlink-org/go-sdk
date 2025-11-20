package cache

import (
	"context"

	"github.com/go-redis/cache/v9"
	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidiscompat"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
	db2 "github.com/shortlink-org/go-sdk/db"
	"github.com/shortlink-org/go-sdk/db/drivers/redis"
	"github.com/shortlink-org/go-sdk/observability/metrics"
)

// New returns a new cache.Client.
func New(ctx context.Context, tracer trace.TracerProvider, monitor *metrics.Monitoring, cfg *config.Config) (*cache.Cache, error) {
	cfg.SetDefault("LOCAL_CACHE_TTL", "5m")
	cfg.SetDefault("LOCAL_CACHE_COUNT", 1000)
	cfg.SetDefault("LOCAL_CACHE_METRICS_ENABLED", true)

	store := redis.New(tracer, monitor.Metrics)

	err := store.Init(ctx)
	if err != nil {
		return nil, &InitCacheError{err}
	}

	conn, ok := store.GetConn().(rueidis.Client)
	if !ok {
		return nil, db2.ErrGetConnection
	}

	adapter := &client{
		rueidiscompat.NewAdapter(conn),
	}

	cacheClient := cache.New(&cache.Options{
		Redis:        adapter,
		LocalCache:   cache.NewTinyLFU(cfg.GetInt("LOCAL_CACHE_COUNT"), cfg.GetDuration("LOCAL_CACHE_TTL")),
		StatsEnabled: cfg.GetBool("LOCAL_CACHE_METRICS_ENABLED"),
		Marshal:      nil,
		Unmarshal:    nil,
	})

	return cacheClient, nil
}
