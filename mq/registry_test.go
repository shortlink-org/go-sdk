//go:build unit

package mq

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/config"
)

func testDeps(t *testing.T) (*slog.Logger, *config.Config) {
	t.Helper()

	log := slog.New(slog.DiscardHandler)

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

// withCleanRegistry isolates a test that registers drivers: the registry and the
// recorded registration errors are process-wide, so a test that dirties them
// would fail every New that runs after it.
func withCleanRegistry(t *testing.T) {
	t.Helper()

	registryMu.Lock()
	savedRegistry := maps.Clone(registry)
	savedErrs := registerErrs
	registryMu.Unlock()

	t.Cleanup(func() {
		registryMu.Lock()
		defer registryMu.Unlock()

		registry = savedRegistry
		registerErrs = savedErrs
	})
}

// TestRegisterRejectsDuplicates - a name collision is reported to the caller of
// New instead of taking the process down from init.
func TestRegisterRejectsDuplicates(t *testing.T) {
	withCleanRegistry(t)

	factory := func(Deps) (MQ, error) { return nil, nil } //nolint:nilnil // test stub

	Register("test-duplicate", factory)
	require.NoError(t, RegistrationError(), "the first registration is accepted")

	require.NotPanics(t, func() { Register("test-duplicate", factory) })

	var registerErr *RegisterError
	require.ErrorAs(t, RegistrationError(), &registerErr, "expected RegisterError")
	require.Equal(t, "test-duplicate", registerErr.Driver)
}

// TestRegisterRejectsNilFactory - a nil factory is reported the same way, rather
// than being stored and panicking later on use.
func TestRegisterRejectsNilFactory(t *testing.T) {
	withCleanRegistry(t)

	require.NotPanics(t, func() { Register("test-nil", nil) })

	var registerErr *RegisterError
	require.ErrorAs(t, RegistrationError(), &registerErr, "expected RegisterError")
	require.Equal(t, "test-nil", registerErr.Driver)

	_, ok := lookup("test-nil")
	require.False(t, ok, "a rejected driver is not registered")
}

// TestNewReportsRegistrationFailure - New surfaces the recorded failure instead
// of reporting the driver as merely unknown.
func TestNewReportsRegistrationFailure(t *testing.T) {
	withCleanRegistry(t)

	ctx := context.Background()
	log, cfg := testDeps(t)

	Register("test-broken", nil)

	cfg.Set("MQ_ENABLED", true)

	bus, err := New(ctx, log, cfg)
	require.Nil(t, bus)

	var registerErr *RegisterError
	require.ErrorAs(t, err, &registerErr, "expected RegisterError")
}
