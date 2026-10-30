package es_postgres

import (
	"errors"
	"fmt"
)

var (
	// ErrConcurrencyConflict is matched by the error Save returns when the
	// aggregate moved past the version the events were built on. Reload the
	// aggregate and retry the command.
	ErrConcurrencyConflict = errors.New("eventsourcing: concurrency conflict")

	// ErrInvalidEvent is returned for a nil event or a version below 1.
	ErrInvalidEvent = errors.New("eventsourcing: invalid event")

	// ErrInvalidAggregateID is returned when an aggregate id is not a UUID.
	ErrInvalidAggregateID = errors.New("eventsourcing: aggregate id is not a UUID")

	// ErrInvalidEventID is returned when an event id is set and is not a UUID.
	ErrInvalidEventID = errors.New("eventsourcing: event id is not a UUID")
)

// ConcurrencyConflictError carries the details of an ErrConcurrencyConflict.
type ConcurrencyConflictError struct {
	// Err is the database error behind the conflict, if there is one.
	Err         error
	AggregateID string
	Version     int32
	Updated     int64
}

func (e *ConcurrencyConflictError) Error() string {
	message := fmt.Sprintf("%s: aggregate %s at version %d. Updated: %d/1", ErrConcurrencyConflict, e.AggregateID, e.Version, e.Updated)
	if e.Err != nil {
		return message + ": " + e.Err.Error()
	}

	return message
}

// Is reports whether target is ErrConcurrencyConflict.
func (*ConcurrencyConflictError) Is(target error) bool {
	return target == ErrConcurrencyConflict //nolint:errorlint // this is the errors.Is contract
}

func (e *ConcurrencyConflictError) Unwrap() error {
	return e.Err
}

// IncorrectUpdatedBillingError is the former name of ConcurrencyConflictError.
//
// Deprecated: use ErrConcurrencyConflict or ConcurrencyConflictError.
type IncorrectUpdatedBillingError = ConcurrencyConflictError
