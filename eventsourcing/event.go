package eventsourcing

import (
	"encoding/json"
	"fmt"
	"time"
)

// Event is a change that happened to an aggregate.
type Event struct {
	// ID is a UUID. The store generates one when it is empty.
	ID string
	// AggregateID and AggregateType name the aggregate the event belongs to.
	AggregateID   string
	AggregateType string
	// Version is the version of the aggregate this event brought it to.
	Version int64
	// Type names what happened.
	Type string
	// SchemaVersion is the version of the payload schema, what an Upcaster keys on.
	// The store writes 1 when it is 0.
	SchemaVersion int
	// Payload is the body of the event, as JSON.
	Payload json.RawMessage
	// CorrelationID is shared by everything one request caused.
	CorrelationID string
	// CausationID is the id of the command or event that caused this event.
	CausationID string
	// Metadata holds free-form pairs. The store adds the W3C trace context of the write.
	Metadata map[string]string
	// CreatedAt is set by the store when it is zero.
	CreatedAt time.Time
	// Cursor is the position of the event in the global stream, set on read.
	Cursor string
}

// NewEvent returns an event of the given type with the payload as JSON.
func NewEvent(eventType string, payload any) (*Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}

	return &Event{Type: eventType, Payload: raw}, nil
}

// Snapshot is the state of an aggregate at a version.
type Snapshot struct {
	AggregateID   string
	AggregateType string
	Version       int64
	// SchemaVersion is the version of the payload schema. The store writes 1 when it is 0.
	SchemaVersion int
	Payload       json.RawMessage
	// CreatedAt and UpdatedAt are set by the store.
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AggregateRef names an aggregate and its current version.
type AggregateRef struct {
	ID      string
	Type    string
	Version int64
}
