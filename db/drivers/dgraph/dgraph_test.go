//go:build unit || (database && dgraph)

package dgraph

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

// A Dgraph cluster is two processes: zero assigns the tablets, alpha serves
// the data. They find each other over a private network, so both need an
// alias that resolves from the other container.
const (
	dgraphImage = "dgraph/dgraph:latest"
	zeroAlias   = "test-dgraph-zero"
	alphaAlias  = "test-dgraph-alpha"
)

func TestMain(m *testing.M) {
	// The dns watcher ignore is not cosmetic: dgo v250.0.0 builds its client
	// without recording the connections it opened, so Dgraph.Close closes
	// nothing and one gRPC connection survives per client created — one per
	// Init attempt, and this test retries. Nothing in the driver can reach
	// those connections. Drop this line once dgo assigns conns, and the test
	// will say whether it has.
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("google.golang.org/grpc/internal/grpcsync.(*CallbackSerializer).run"),
		goleak.IgnoreTopFunction("google.golang.org/grpc.(*addrConn).resetTransport"),
		goleak.IgnoreTopFunction("google.golang.org/grpc/internal/resolver/dns.(*dnsResolver).watcher"))

	os.Exit(m.Run())
}

func TestDgraph(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)

	logConf := logger.Configuration{
		Level: logger.INFO_LEVEL,
	}
	log, err := logger.New(logConf)
	require.NoError(t, err)
	t.Cleanup(func() {
		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = log.Close()
	})

	store := New(log, cfg)

	nw, err := network.New(ctx)
	require.NoError(t, err)

	zero, err := testcontainers.Run(ctx, dgraphImage,
		network.WithNetwork([]string{zeroAlias}, nw),
		testcontainers.WithCmd("dgraph", "zero", "--my="+zeroAlias+":5080"),
		testcontainers.WithExposedPorts("5080/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5080/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	require.NoError(t, err)

	// --my has to be the address the other node can dial. Alpha announcing
	// itself as localhost sends zero to its own container instead.
	alpha, err := testcontainers.Run(ctx, dgraphImage,
		network.WithNetwork([]string{alphaAlias}, nw),
		// Alter is guarded by an IP whitelist that defaults to localhost, and
		// the client reaches alpha from the bridge address, so the schema
		// migration is refused without this. Test-only: it opens admin
		// operations to anything that can reach the port.
		testcontainers.WithCmd("dgraph", "alpha", "--my="+alphaAlias+":7080", "--zero="+zeroAlias+":5080",
			"--security", "whitelist=0.0.0.0/0"),
		testcontainers.WithExposedPorts("9080/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("9080/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	if err != nil {
		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = zero.Terminate(context.Background())
		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = nw.Remove(context.Background())
	}

	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = alpha.Terminate(context.Background())
		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = zero.Terminate(context.Background())
		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = nw.Remove(context.Background())
	})

	host, err := alpha.Host(ctx)
	require.NoError(t, err)
	mapped, err := alpha.MappedPort(ctx, "9080/tcp")
	require.NoError(t, err)

	t.Setenv("STORE_DGRAPH_URI", fmt.Sprintf("%s:%s", host, mapped.Port()))

	// Alpha reports the port open before it will serve an Alter, so Init is
	// retried. Keeping the last error makes a failure say why, instead of
	// spending two minutes to report only that it never succeeded.
	var lastErr error

	ok := assert.Eventually(t, func() bool {
		lastErr = store.Init(ctx)

		return lastErr == nil
	}, 2*time.Minute, time.Second, "dgraph init")

	if !ok {
		require.NoError(t, lastErr, "dgraph never accepted the schema migration")
	}
}
