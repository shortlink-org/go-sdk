//go:build unit || (database && etcd)

package etcd

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcetcd "github.com/testcontainers/testcontainers-go/modules/etcd"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestETCD(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	etcdC, err := tcetcd.Run(ctx, "gcr.io/etcd-development/etcd:v3.5.14")
	testcontainers.CleanupContainer(t, etcdC)
	require.NoError(t, err, "etcd container: ensure Docker is running")

	t.Cleanup(cancel)

	endpoint, err := etcdC.ClientEndpoint(ctx)
	require.NoError(t, err)
	// client/v3 Endpoints expect host:port (see setConfig); ClientEndpoint returns http://host:port
	hostPort := strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	cfg.Set("STORE_ETCD_URI", hostPort)
	require.NoError(t, store.Init(ctx))
}
