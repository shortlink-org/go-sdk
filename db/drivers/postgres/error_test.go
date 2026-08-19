//go:build unit

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/config"
)

var errCause = errors.New("underlying")

// A StoreError has to answer errors.Is about two different things at once: the
// sentinel, so a caller can decide what to do, and the cause, so a log line
// can say what happened. Losing either makes the type pointless.
func TestStoreErrorCarriesKindAndCause(t *testing.T) {
	t.Parallel()

	err := storeError(opConnect, ErrInvalidCredentials, errCause, "failed to open the database")

	require.ErrorIs(t, err, ErrInvalidCredentials, "the kind must be reachable")
	require.ErrorIs(t, err, errCause, "the cause must be reachable")
	assert.Contains(t, err.Error(), "connect")
	assert.Contains(t, err.Error(), "failed to open the database")
}

func TestStoreErrorTolerantOfMissingParts(t *testing.T) {
	t.Parallel()

	kindOnly := storeError(opConfig, ErrInvalidDSN, nil, "")
	require.ErrorIs(t, kindOnly, ErrInvalidDSN)

	causeOnly := storeError(opConfig, nil, errCause, "")
	require.ErrorIs(t, causeOnly, errCause)

	assert.NotPanics(t, func() { _ = storeError(opConfig, nil, nil, "").Error() })
}

// The classification exists so that a caller can tell "the password is wrong"
// from "the server is down" without matching message text, which is localized
// and changes between releases.
func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "missing database",
			err:  &pgconn.PgError{Code: pgerrcode.InvalidCatalogName},
			want: ErrInvalidDatabase,
		},
		{
			name: "wrong password",
			err:  &pgconn.PgError{Code: pgerrcode.InvalidPassword},
			want: ErrInvalidCredentials,
		},
		{
			name: "rejected authorization",
			err:  &pgconn.PgError{Code: pgerrcode.InvalidAuthorizationSpecification},
			want: ErrInvalidCredentials,
		},
		{
			name: "missing schema",
			err:  &pgconn.PgError{Code: pgerrcode.InvalidSchemaName},
			want: ErrInvalidSchema,
		},
		{
			name: "a code we have no opinion about",
			err:  &pgconn.PgError{Code: pgerrcode.SyntaxError},
			want: ErrClientConnection,
		},
		{
			// Not a server error at all — a refused dial, say. We know the
			// connection did not come up, not why.
			name: "not a PostgreSQL error",
			err:  errCause,
			want: ErrClientConnection,
		},
		{
			name: "wrapped",
			err:  errors.Join(errCause, &pgconn.PgError{Code: pgerrcode.InvalidPassword}),
			want: ErrInvalidCredentials,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, classify(tt.err))
		})
	}
}

// Without Unwrap, a caller cannot recognize a canceled context behind a
// failed ping — which is the one case where retrying is pointless.
func TestPingConnectionErrorUnwraps(t *testing.T) {
	t.Parallel()

	err := &PingConnectionError{Err: context.Canceled}

	require.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "failed to ping the database")

	assert.NotPanics(t, func() { _ = (&PingConnectionError{}).Error() })
}

// The DSN sentinel is documented as reachable from Init; this is the path that
// has to keep producing it.
func TestInitReportsAnUnparseableDSN(t *testing.T) {
	t.Parallel()

	cfg, err := config.New()
	require.NoError(t, err)

	cfg.Set(cfgURI, "://not-a-dsn")

	store := New(nil, nil, cfg)

	err = store.Init(t.Context())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidDSN)

	var storeErr *StoreError
	require.ErrorAs(t, err, &storeErr)
	assert.Equal(t, opConfig, storeErr.Op)
}
