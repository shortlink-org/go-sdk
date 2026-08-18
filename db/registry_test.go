//go:build unit

package db

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

func testDeps(t *testing.T) (logger.Logger, *config.Config) {
	t.Helper()

	log, err := logger.New(logger.Configuration{})
	require.NoError(t, err, "Error init a logger")

	cfg, err := config.New()
	require.NoError(t, err, "Error init config")

	return log, cfg
}

// TestRamIsBuiltIn - the default store type resolves without any driver import.
func TestRamIsBuiltIn(t *testing.T) {
	require.True(t, slices.Contains(Drivers(), defaultStoreType),
		"the default store type must always be registered")
}

// TestUnknownStoreTypeFails - an unknown store type is an error, not a silent
// fallback to the in-memory store.
func TestUnknownStoreTypeFails(t *testing.T) {
	ctx := context.Background()
	log, cfg := testDeps(t)

	cfg.Set("STORE_TYPE", "postgress") // typo on purpose

	store, err := New(ctx, log, nil, nil, cfg)
	require.Nil(t, store, "no store must be returned for an unknown type")

	var unknownErr *UnknownStoreTypeError
	require.ErrorAs(t, err, &unknownErr, "expected UnknownStoreTypeError")
	require.Equal(t, "postgress", unknownErr.StoreType)
	require.Contains(t, unknownErr.Error(), defaultStoreType,
		"the message must list the registered drivers")
}

// TestDriverOptionTypeMismatch - an option addressed to a driver that cannot
// use it is reported instead of being dropped.
func TestDriverOptionTypeMismatch(t *testing.T) {
	type wantedOption func()

	deps := Deps{
		driver:  "example",
		options: []any{"not an option"},
	}

	opts, err := DriverOptions[wantedOption](deps)
	require.Nil(t, opts)

	var optErr *DriverOptionTypeError
	require.ErrorAs(t, err, &optErr, "expected DriverOptionTypeError")
	require.Equal(t, "example", optErr.Driver)
}

// TestConnReportsWrongType - Conn reports ErrGetConnection when the driver in
// use is not the expected one.
func TestConnReportsWrongType(t *testing.T) {
	ctx := context.Background()
	log, cfg := testDeps(t)

	cfg.Set("STORE_TYPE", defaultStoreType)

	store, err := New(ctx, log, nil, nil, cfg)
	require.NoError(t, err, "Error init a db")

	_, err = Conn[*testing.T](store)
	require.True(t, errors.Is(err, ErrGetConnection), "expected ErrGetConnection")
}

// TestOptionsForUnregisteredDriverFail - an option whose target nothing
// answers to is reported, instead of being carried and never applied.
func TestOptionsForUnregisteredDriverFail(t *testing.T) {
	ctx := context.Background()
	log, cfg := testDeps(t)

	store, err := New(ctx, log, nil, nil, cfg,
		DriverOption("postgress", "whatever"), // typo on purpose
	)
	require.Nil(t, store)

	var targetErr *UnknownOptionTargetError
	require.ErrorAs(t, err, &targetErr, "expected UnknownOptionTargetError")
	require.Equal(t, "postgress", targetErr.Driver)
}

// TestOptionsForUnselectedDriverIgnored - options for a registered driver that
// is not the selected one are legitimate and must not fail New.
func TestOptionsForUnselectedDriverIgnored(t *testing.T) {
	ctx := context.Background()
	log, cfg := testDeps(t)

	cfg.Set("STORE_TYPE", defaultStoreType)

	store, err := New(ctx, log, nil, nil, cfg,
		DriverOption(defaultStoreType, "ignored by the ram factory"),
	)
	require.NoError(t, err)
	require.NotNil(t, store)
}
