//go:build unit || (database && mongo)

package mongo

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestMongo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	mongoContainer, err := mongodb.Run(ctx, "mongo:7")
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = mongoContainer.Terminate(context.Background())
	})

	uri, err := mongoContainer.ConnectionString(ctx)
	require.NoError(t, err)
	t.Setenv("STORE_MONGODB_URI", strings.TrimSuffix(uri, "/")+"/shortlink")
	require.NoError(t, store.Init(ctx))
}
