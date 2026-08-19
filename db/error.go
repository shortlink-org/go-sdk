package db

import (
	"errors"
	"fmt"
	"strings"
)

// ErrGetConnection - error gets connection
var ErrGetConnection = errors.New("error get connection")

// RegisterError - a driver could not be registered.
//
// Register runs from init, where there is no caller to hand an error to, so the
// problem is recorded and reported by New.
type RegisterError struct {
	Driver string
	Reason string
}

func (e *RegisterError) Error() string {
	return "db: cannot register driver " + e.Driver + ": " + e.Reason
}

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

// StoreError reports a failure during one phase of a driver's lifecycle.
//
// Every driver used to carry its own copy of this type. That was not merely
// repetitive: a defect had to be fixed sixteen times to be fixed at all, and
// in practice was not — six of the seven drivers with a ping error were
// missing Unwrap, so a caller could not see a canceled context behind a
// failed ping.
type StoreError struct {
	// Driver names the store the failure came from. An empty value omits the
	// prefix, so a driver that has not been updated still reads sensibly.
	Driver  string
	Op      string
	Err     error
	Details string
}

// Error implements the error interface.
func (e *StoreError) Error() string {
	subject := "store error"
	if e.Driver != "" {
		subject = e.Driver + " store error"
	}

	if e.Details != "" {
		return fmt.Sprintf("%s during %s: %s: %v", subject, e.Op, e.Details, e.Err)
	}

	return fmt.Sprintf("%s during %s: %v", subject, e.Op, e.Err)
}

// Unwrap allows errors.Is and errors.As to reach the cause.
func (e *StoreError) Unwrap() error {
	return e.Err
}

// PingConnectionError reports that a connection was established but the server
// did not answer a ping.
type PingConnectionError struct {
	Driver string
	Err    error
}

// Error implements the error interface.
func (e *PingConnectionError) Error() string {
	subject := "failed to ping the database"
	if e.Driver != "" {
		subject = "failed to ping " + e.Driver
	}

	if e.Err == nil {
		return subject
	}

	return subject + ": " + e.Err.Error()
}

// Unwrap reaches the cause, so that a caller can still recognize a canceled
// context or a driver-specific error behind a failed ping.
func (e *PingConnectionError) Unwrap() error {
	return e.Err
}
