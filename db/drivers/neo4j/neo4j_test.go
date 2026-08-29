//go:build unit || (database && neo4j)

package neo4j

import (
	"context"
	"net/url"
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
	const password = "shortlink-test"

	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)

	store := New(cfg)

	neo4jContainer, err := neo4j.Run(ctx, "neo4j:5-community", neo4j.WithAdminPassword(password))
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = neo4jContainer.Terminate(context.Background())
	})

	boltURL, err := neo4jContainer.BoltUrl(ctx)
	require.NoError(t, err)

	uri, err := url.Parse(boltURL)
	require.NoError(t, err)
	uri.User = url.UserPassword("neo4j", password)

	t.Setenv("STORE_NEO4J_URI", uri.String())
	require.NoError(t, store.Init(ctx))
}

func TestSetConfigPreservesCredentials(t *testing.T) {
	cfg, err := config.New()
	require.NoError(t, err)

	cfg.Set("STORE_NEO4J_URI", "neo4j://alice:s3cr%40t@example.com:7687")
	store := New(cfg)

	require.NoError(t, store.setConfig())
	require.Equal(t, "neo4j://example.com:7687", store.config.URI)
	require.Equal(t, "alice", store.config.login)
	require.Equal(t, "s3cr@t", store.config.password)
}
