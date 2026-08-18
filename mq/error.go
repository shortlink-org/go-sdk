package mq

import (
	"errors"
	"strings"
)

// ErrDisabled - MQ is switched off through MQ_ENABLED.
//
// New reports it instead of returning a nil DataBus with a nil error: a nil
// return is easy to forget to check, and every method on it panics.
var ErrDisabled = errors.New("mq: disabled through MQ_ENABLED")

// RegisterError - a driver could not be registered.
//
// Register runs from init, where there is no caller to hand an error to, so the
// problem is recorded and reported by New.
type RegisterError struct {
	Driver string
	Reason string
}

func (e *RegisterError) Error() string {
	return "mq: cannot register driver " + e.Driver + ": " + e.Reason
}

// UnknownMQTypeError - unknown MQ type error
type UnknownMQTypeError struct {
	MQType string

	// Registered lists the drivers available at the time of the failure, so
	// the message can tell a typo apart from a driver that was never imported.
	Registered []string
}

func (e *UnknownMQTypeError) Error() string {
	registered := "none"
	if len(e.Registered) > 0 {
		registered = strings.Join(e.Registered, ", ")
	}

	return "unknown mq type: " + e.MQType +
		" (registered: " + registered +
		"); import the driver for its side effect: " +
		`import _ "github.com/shortlink-org/go-sdk/mq/` + e.MQType + `"`
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

// DriverOptionTypeError - an option was addressed to a driver that cannot use it
type DriverOptionTypeError struct {
	Driver string
	Want   string
	Got    string
}

func (e *DriverOptionTypeError) Error() string {
	return "driver " + e.Driver + ": expected option of type " + e.Want + ", got " + e.Got
}
