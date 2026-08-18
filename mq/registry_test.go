//go:build unit

package mq

import (
	"context"
	"errors"
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

// TestDisabledIsAnError - a switched-off bus is reported, not handed back as a
// nil DataBus whose every method panics.
func TestDisabledIsAnError(t *testing.T) {
	ctx := context.Background()
	log, cfg := testDeps(t)

	cfg.Set("MQ_ENABLED", false)

	bus, err := New(ctx, log, cfg)
	require.Nil(t, bus)
	require.True(t, errors.Is(err, ErrDisabled), "expected ErrDisabled")
}

// TestUnknownMQTypeFails - an unknown MQ_TYPE is an error, not a silent
// fallback to whichever driver the switch happened to list last.
func TestUnknownMQTypeFails(t *testing.T) {
	ctx := context.Background()
	log, cfg := testDeps(t)

	cfg.Set("MQ_ENABLED", true)
	cfg.Set("MQ_TYPE", "rabitmq") // typo on purpose

	bus, err := New(ctx, log, cfg)
	require.Nil(t, bus)

	var unknownErr *UnknownMQTypeError
	require.ErrorAs(t, err, &unknownErr, "expected UnknownMQTypeError")
	require.Equal(t, "rabitmq", unknownErr.MQType)
}

// TestOptionsForUnregisteredDriverFail - an option whose target nothing
// answers to is reported, instead of being carried and never applied.
func TestOptionsForUnregisteredDriverFail(t *testing.T) {
	ctx := context.Background()
	log, cfg := testDeps(t)

	cfg.Set("MQ_ENABLED", true)

	bus, err := New(ctx, log, cfg, DriverOption("rabitmq", "whatever"))
	require.Nil(t, bus)

	var targetErr *UnknownOptionTargetError
	require.ErrorAs(t, err, &targetErr, "expected UnknownOptionTargetError")
	require.Equal(t, "rabitmq", targetErr.Driver)
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

// TestRegisterRejectsDuplicates - a name collision surfaces at startup rather
// than silently shadowing a driver.
func TestRegisterRejectsDuplicates(t *testing.T) {
	factory := func(Deps) (MQ, error) { return nil, nil } //nolint:nilnil // test stub

	Register("test-duplicate", factory)
	require.Panics(t, func() { Register("test-duplicate", factory) })
}
