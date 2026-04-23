//go:build unit || (database && redis)

package cache_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	cache2 "github.com/go-redis/cache/v9"
	"github.com/shortlink-org/go-sdk/cache"
	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/observability/metrics"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestCache(t *testing.T) {
	ctx := context.Background()

	c, err := testcontainers.Run(ctx, "redis:7-alpine",
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("6379/tcp").WithStartupTimeout(5*time.Minute),
		),
	)
	require.NoError(t, err, "Could not start redis container")

	t.Cleanup(func() {
		_ = c.Terminate(context.Background())
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	redisAddr := fmt.Sprintf("%s:%s", host, mapped.Port())

	require.Eventually(t, func() bool {
		cfg, errCfg := config.New()
		if errCfg != nil {
			return false
		}
		cfg.Set("STORE_REDIS_URI", redisAddr)
		tryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, errCache := cache.New(tryCtx, nil, &metrics.Monitoring{}, cfg)
		return errCache == nil
	}, 5*time.Minute, time.Second, "redis ready for cache")

	t.Run("Test Set and Get", func(t *testing.T) {
		cfg, err := config.New()
		require.NoError(t, err)
		cfg.Set("STORE_REDIS_URI", redisAddr)

		cClient, err := cache.New(context.Background(), nil, &metrics.Monitoring{}, cfg)
		require.NoError(t, err)

		key := "myKey"
		value := "myValue"

		err = cClient.Set(&cache2.Item{
			Key:   key,
			Value: value,
		})
		require.NoError(t, err)

		resp := ""
		err = cClient.Get(context.Background(), key, &resp)
		require.NoError(t, err)
		require.Equal(t, value, resp)
	})
}
