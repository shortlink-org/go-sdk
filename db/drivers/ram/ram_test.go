//go:build unit || (database && ram)

package ram

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestRAM(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// InitStore
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	err = store.Init(ctx)
	require.NoError(t, err)

	// Run tests
	t.Cleanup(func() {
		cancel()
	})
}
