//go:build unit || (database && badger)

package badger

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("github.com/golang/glog.(*fileSink).flushDaemon"))

	os.Exit(m.Run())
}

func TestBadger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	err := store.Init(ctx)
	require.NoError(t, err)

	t.Run("Close", func(t *testing.T) {
		cancel()
	})
}
