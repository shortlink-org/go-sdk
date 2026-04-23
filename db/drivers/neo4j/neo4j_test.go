//go:build unit || (database && neo4j)

package neo4j

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/neo4j"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestNeo4j(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	neo4jContainer, err := neo4j.Run(ctx, "neo4j:5-community")
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = neo4jContainer.Terminate(context.Background())
	})

	boltURL, err := neo4jContainer.BoltUrl(ctx)
	require.NoError(t, err)
	t.Setenv("STORE_NEO4J_URI", boltURL)
	require.NoError(t, store.Init(ctx))
}
