//go:build unit

package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/db"
)

var errCause = errors.New("underlying")

// Every driver aliases these types, so a caller matching on *db.StoreError has
// to keep working whichever store produced the error.
func TestStoreErrorMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *db.StoreError
		want string
	}{
		{
			name: "named driver, with details",
			err:  &db.StoreError{Driver: "postgres", Op: "connect", Err: errCause, Details: "failed to open"},
			want: "postgres store error during connect: failed to open: underlying",
		},
		{
			name: "named driver, no details",
			err:  &db.StoreError{Driver: "redis", Op: "config", Err: errCause},
			want: "redis store error during config: underlying",
		},
		{
			// A driver that has not set Driver still reads sensibly rather
			// than emitting a stray leading space.
			name: "unnamed driver",
			err:  &db.StoreError{Op: "connect", Err: errCause},
			want: "store error during connect: underlying",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

func TestStoreErrorUnwraps(t *testing.T) {
	t.Parallel()

	err := &db.StoreError{Driver: "mysql", Op: "connect", Err: errCause}

	require.ErrorIs(t, err, errCause)
	assert.NotPanics(t, func() { _ = (&db.StoreError{}).Error() })
}

// Without Unwrap a caller cannot recognize a canceled context behind a failed
// ping — which is the one case where retrying is pointless. Six of the seven
// drivers that had their own copy of this type were missing it.
func TestPingConnectionError(t *testing.T) {
	t.Parallel()

	err := &db.PingConnectionError{Driver: "mongo", Err: context.Canceled}

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "failed to ping mongo: context canceled", err.Error())

	assert.Equal(t, "failed to ping the database", (&db.PingConnectionError{}).Error())
	assert.NotPanics(t, func() { _ = (&db.PingConnectionError{Driver: "etcd"}).Error() })
}
