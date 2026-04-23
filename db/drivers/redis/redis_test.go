//go:build unit || (database && redis)

package redis

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestRedis(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(nil, nil, cfg)

	c, err := tcredis.Run(ctx, "redis:7-alpine")
	testcontainers.CleanupContainer(t, c)
	require.NoError(t, err)

	t.Cleanup(cancel)

	uri, err := c.ConnectionString(ctx)
	require.NoError(t, err)
	u, err := url.Parse(uri)
	require.NoError(t, err)
	cfg.Set("STORE_REDIS_URI", u.Host)
	require.NoError(t, store.Init(ctx))
}
