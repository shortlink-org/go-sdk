package db

import (
	"context"

	"github.com/shortlink-org/go-sdk/config"
)

// defaultStoreType is used when STORE_TYPE is unset or empty.
const defaultStoreType = "ram"

// DB - common interface of db
type DB interface {
	Init(ctx context.Context) error
	GetConn() any
}

// Store abstract type
type Store struct {
	DB

	typeStore string
	cfg       *config.Config
}

// Conn returns the driver connection as T.
//
// It replaces the type assertion every caller used to write by hand, and
// reports ErrGetConnection when the driver in use is not the expected one:
//
//	pool, err := db.Conn[*pgxpool.Pool](store)
func Conn[T any](d DB) (T, error) {
	conn, ok := d.GetConn().(T)
	if !ok {
		var zero T

		return zero, ErrGetConnection
	}

	return conn, nil
}

// Options holds the options collected for each driver.
type Options struct {
	// driverOptions maps a driver name to the options addressed to it. They
	// stay untyped here so that this package needs no import of any driver.
	driverOptions map[string][]any
}

// Option is a functional option for configuring database.
type Option func(*Options)

// DriverOption addresses options to a driver by name.
//
// It is the seam a driver uses to expose its own typed constructor, and is not
// meant to be called directly: the driver owns its name, so client code never
// spells it out. See postgres.With for the shape a driver should offer.
//
// Options reach the driver only when it is the one selected by STORE_TYPE;
// options for another registered driver are ignored, which lets one binary
// carry the wiring for several deployments. Options addressed to a driver that
// is not registered fail db.New, so a wrong name cannot pass unnoticed.
func DriverOption(driver string, opts ...any) Option {
	return func(o *Options) {
		o.driverOptions[driver] = append(o.driverOptions[driver], opts...)
	}
}
