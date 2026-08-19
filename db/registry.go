package db

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db/drivers/ram"
	"github.com/shortlink-org/go-sdk/logger"
)

// Deps carries the shared dependencies handed to every driver factory.
//
// Driver-specific options travel alongside them and stay opaque to this
// package; a driver unwraps its own with DriverOptions.
type Deps struct {
	Log     logger.Logger
	Tracer  trace.TracerProvider
	Metrics *metric.MeterProvider
	Cfg     *config.Config

	driver  string
	options []any
}

// Factory builds a driver from the shared dependencies.
//
//nolint:gocritic // Deps is public API; every driver's init depends on this shape
type Factory func(deps Deps) (DB, error)

//nolint:gochecknoglobals // driver registry, mirrors database/sql
var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}

	// Failures collected by Register, for New to report. Not a sentinel: it is
	// whatever the drivers could not agree on, joined.
	errRegistration error
)

// Register makes a driver available under name. Drivers call it from init, so
// a blank import of the driver package is what makes the driver selectable:
//
//	import _ "github.com/shortlink-org/go-sdk/db/drivers/postgres"
//
// A nil factory or a duplicate name is recorded instead of raised: Register runs
// from init, where there is no caller to hand an error to and a panic would take
// the process down before main. New reports what was recorded, so the wiring
// mistake still surfaces — as an error the caller decides what to do with.
func Register(name string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if factory == nil {
		errRegistration = errors.Join(errRegistration, &RegisterError{Driver: name, Reason: "factory is nil"})

		return
	}

	if _, dup := registry[name]; dup {
		errRegistration = errors.Join(errRegistration, &RegisterError{Driver: name, Reason: "registered twice"})

		return
	}

	registry[name] = factory
}

// RegistrationError reports what Register could not accept, or nil when every
// driver registered cleanly. New calls it, so checking it separately is only
// needed to inspect the registry before building a store.
func RegistrationError() error {
	registryMu.RLock()
	defer registryMu.RUnlock()

	return errRegistration
}

// Drivers returns the names of the registered drivers, sorted.
func Drivers() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// lookup resolves a driver factory by name.
func lookup(name string) (Factory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	factory, ok := registry[name]

	return factory, ok
}

// DriverOptions converts the options addressed to this driver into the
// driver's own option type. An option of the wrong type is a wiring mistake
// and is reported rather than ignored.
//
//nolint:gocritic // Deps is public API; drivers call this from their factory
func DriverOptions[T any](deps Deps) ([]T, error) {
	opts := make([]T, 0, len(deps.options))

	for _, raw := range deps.options {
		opt, ok := raw.(T)
		if !ok {
			var want T

			return nil, &DriverOptionTypeError{
				Driver: deps.driver,
				Want:   fmt.Sprintf("%T", want),
				Got:    fmt.Sprintf("%T", raw),
			}
		}

		opts = append(opts, opt)
	}

	return opts, nil
}

// ram is dependency-free and backs the default STORE_TYPE, so it is built in
// rather than opt-in. Registering it here (instead of in the driver package)
// keeps drivers/ram free of an import back into this package.
//
//nolint:gochecknoinits // driver registration
func init() {
	Register(defaultStoreType, func(deps Deps) (DB, error) {
		return ram.New(deps.Cfg), nil
	})
}
