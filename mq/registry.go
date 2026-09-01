package mq

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/shortlink-org/go-sdk/config"
)

// Deps carries the shared dependencies handed to every driver factory.
//
// Driver-specific options travel alongside them and stay opaque to this
// package; a driver unwraps its own with DriverOptions.
type Deps struct {
	Log *slog.Logger
	Cfg *config.Config

	driver  string
	options []any
}

// Factory builds a driver from the shared dependencies.
type Factory func(deps Deps) (MQ, error)

//nolint:gochecknoglobals // driver registry, mirrors database/sql
var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}

	// registerErrs collects what Register could not accept, for New to report.
	registerErrs error
)

// Register makes a driver available under name. Drivers call it from init, so
// a blank import of the driver package is what makes the driver selectable:
//
//	import _ "github.com/shortlink-org/go-sdk/mq/kafka"
//
// A nil factory or a duplicate name is recorded instead of raised: Register runs
// from init, where there is no caller to hand an error to and a panic would take
// the process down before main. New reports what was recorded, so the wiring
// mistake still surfaces — as an error the caller decides what to do with.
func Register(name string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if factory == nil {
		registerErrs = errors.Join(registerErrs, &RegisterError{Driver: name, Reason: "factory is nil"})

		return
	}

	if _, dup := registry[name]; dup {
		registerErrs = errors.Join(registerErrs, &RegisterError{Driver: name, Reason: "registered twice"})

		return
	}

	registry[name] = factory
}

// RegistrationError reports what Register could not accept, or nil when every
// driver registered cleanly. New calls it, so checking it separately is only
// needed to inspect the registry before building a bus.
func RegistrationError() error {
	registryMu.RLock()
	defer registryMu.RUnlock()

	return registerErrs
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

// DriverOption addresses options to a driver by name.
//
// It is the seam a driver uses to expose its own typed constructor, and is not
// meant to be called directly: the driver owns its name, so client code never
// spells it out.
//
// Options reach the driver only when it is the one selected by MQ_TYPE.
// Options addressed to a driver that is not registered fail New, so a wrong
// name cannot pass unnoticed.
func DriverOption(driver string, opts ...any) Option {
	return func(o *Options) {
		o.driverOptions[driver] = append(o.driverOptions[driver], opts...)
	}
}

// DriverOptions converts the options addressed to this driver into the
// driver's own option type. An option of the wrong type is a wiring mistake
// and is reported rather than ignored.
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
