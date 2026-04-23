package sync_map_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/concurrency/sync_map"
)

func Test_SyncMap(t *testing.T) {
	t.Parallel()

	syncMap := sync_map.New()

	for i := range 1000 {
		syncMap.Set(i, "value")
	}

	require.Equal(t, "value", syncMap.Get(5))
}
