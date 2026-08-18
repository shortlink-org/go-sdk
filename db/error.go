package db

import (
	"errors"
	"strings"
)

// ErrGetConnection - error gets connection
var ErrGetConnection = errors.New("error get connection")

// UnknownStoreTypeError - unknown store type error
type UnknownStoreTypeError struct {
	StoreType string

	// Registered lists the drivers available at the time of the failure, so
	// the message can tell a typo apart from a driver that was never imported.
	Registered []string
}

func (e *UnknownStoreTypeError) Error() string {
	registered := "none"
	if len(e.Registered) > 0 {
		registered = strings.Join(e.Registered, ", ")
	}

	return "unknown store type: " + e.StoreType +
		" (registered: " + registered +
		"); import the driver for its side effect: " +
		`import _ "github.com/shortlink-org/go-sdk/db/drivers/` + e.StoreType + `"`
}

// DriverOptionTypeError - an option was addressed to a driver that cannot use it
type DriverOptionTypeError struct {
	Driver string
	Want   string
	Got    string
}

func (e *DriverOptionTypeError) Error() string {
	return "driver " + e.Driver + ": expected option of type " + e.Want + ", got " + e.Got
}

// UnknownOptionTargetError - options were addressed to a driver that is not registered
type UnknownOptionTargetError struct {
	Driver string

	// Registered lists the drivers available at the time of the failure.
	Registered []string
}

func (e *UnknownOptionTargetError) Error() string {
	registered := "none"
	if len(e.Registered) > 0 {
		registered = strings.Join(e.Registered, ", ")
	}

	return "options addressed to unregistered driver " + e.Driver +
		" (registered: " + registered + "); the option would never be applied"
}
