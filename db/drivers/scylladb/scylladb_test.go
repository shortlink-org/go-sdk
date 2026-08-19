//go:build unit || (database && scylladb)

package scylladb

import (
	"context"
	"os"
	"testing"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcscylla "github.com/testcontainers/testcontainers-go/modules/scylladb"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestScyllaDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)

	store := New(cfg)

	c, err := tcscylla.Run(ctx, "scylladb/scylla:6.2")
	testcontainers.CleanupContainer(t, c)
	require.NoError(t, err)

	addr, err := c.NonShardAwareConnectionHost(ctx)
	require.NoError(t, err)

	cfg.Set("STORE_SCYLLADB_HOSTS", addr)
	require.NoError(t, store.Init(ctx))

	t.Cleanup(cancel)

	sess, ok := store.GetConn().(*gocql.Session)
	require.True(t, ok)
	require.NotNil(t, sess)

	var version string

	iter := sess.Query("SELECT release_version FROM system.local").IterContext(ctx)
	iter.Scan(&version)
	require.NoError(t, iter.Close())
	require.NotEmpty(t, version)
}
