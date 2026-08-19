/*
Package db selects a database driver at runtime.

A driver becomes selectable by being imported for its side effect:

	import _ "github.com/shortlink-org/go-sdk/db/drivers/postgres"

Only the drivers actually imported are linked into the binary. The in-memory
"ram" driver is built in and needs no import.
*/
package db

import (
	"context"
	"log/slog"
	"sort"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

// New - return implementation of db
func New(
	ctx context.Context,
	log logger.Logger,
	tracer trace.TracerProvider,
	metrics *metric.MeterProvider,
	cfg *config.Config,
	opts ...Option,
) (*Store, error) {
	// A driver that could not register would otherwise look like one that was
	// never imported, so report the registration failure itself.
	err := RegistrationError()
	if err != nil {
		return nil, err
	}

	options := &Options{driverOptions: make(map[string][]any)}
	for _, opt := range opts {
		opt(options)
	}

	err = checkOptionTargets(options)
	if err != nil {
		return nil, err
	}

	typeStore := storeType(cfg)

	factory, ok := lookup(typeStore)
	if !ok {
		return nil, &UnknownStoreTypeError{
			StoreType:  typeStore,
			Registered: Drivers(),
		}
	}

	driver, err := factory(Deps{
		Log:     log,
		Tracer:  tracer,
		Metrics: metrics,
		Cfg:     cfg,
		driver:  typeStore,
		options: options.driverOptions[typeStore],
	})
	if err != nil {
		return nil, err
	}

	store := &Store{
		DB:        driver,
		typeStore: typeStore,
		cfg:       cfg,
	}

	err = store.Init(ctx)
	if err != nil {
		return nil, err
	}

	log.Info("run db",
		slog.String("db", store.typeStore),
	)

	return store, nil
}

// storeType - resolve the configured store type
func storeType(cfg *config.Config) string {
	cfg.SetDefault("STORE_TYPE", defaultStoreType)

	typeStore := cfg.GetString("STORE_TYPE")
	if typeStore == "" {
		return defaultStoreType
	}

	return typeStore
}

// checkOptionTargets - reject options addressed to a driver that is not
// registered. Options for a registered but unselected driver are legitimate
// and stay silent; a name nothing answers to is a wiring mistake.
func checkOptionTargets(options *Options) error {
	targets := make([]string, 0, len(options.driverOptions))
	for driver := range options.driverOptions {
		targets = append(targets, driver)
	}

	sort.Strings(targets)

	for _, driver := range targets {
		if _, ok := lookup(driver); !ok {
			return &UnknownOptionTargetError{
				Driver:     driver,
				Registered: Drivers(),
			}
		}
	}

	return nil
}
