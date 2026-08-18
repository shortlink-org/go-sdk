//go:build unit || (database && mongo)

package mongo

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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
	// The option has to reach the client options after the defaults are applied
	// and before Connect, so that it can override any of them.
	applied := false
	store := New(cfg, WithClientOptions(func(opts *options.ClientOptions) {
		applied = true

		require.NotNil(t, opts.RetryWrites, "defaults must be applied before the option")

		opts.SetAppName("shortlink-test")
	}))

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
	require.True(t, applied, "WithClientOptions callback was not applied")
}
