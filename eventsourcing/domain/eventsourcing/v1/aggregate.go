package v1

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrNilEvent is returned when a nil event is applied to an aggregate.
	ErrNilEvent = errors.New("eventsourcing: nil event")

	// ErrNilAggregate is returned when a nil aggregate is passed to ApplyChangeHelper.
	ErrNilAggregate = errors.New("eventsourcing: nil aggregate")

	// ErrEventOutOfOrder is returned when replaying an event that is not newer
	// than the aggregate.
	ErrEventOutOfOrder = errors.New("eventsourcing: event is out of order")
)

// AggregateHandler defines the methods to process commands
type AggregateHandler interface {
	ApplyChange(ctx context.Context, event *Event) error
	ApplyChangeHelper(ctx context.Context, aggregate AggregateHandler, event *Event, commit bool) error
	HandleCommand(ctx context.Context, command *BaseCommand) error
	Uncommitted() []*Event
	ClearUncommitted()
	IncrementVersion()
}

// Uncommitted return the events to be saved
func (b *BaseAggregate) Uncommitted() []*Event {
	return b.GetChanges()
}

// ClearUncommitted the events
func (b *BaseAggregate) ClearUncommitted() {
	b.Changes = []*Event{}
}

// IncrementVersion ads 1 to the current version
func (b *BaseAggregate) IncrementVersion() {
	b.Version++
}

// ApplyChangeHelper applies the event and moves the aggregate to its version.
//
// The aggregate argument is the concrete type embedding BaseAggregate: Go has no
// virtual dispatch, so this is how the embedded base reaches the outer ApplyChange.
//
// With commit, the event is new: it takes the next version, ApplyChange sees
// that version, and the event joins the uncommitted changes. Without commit, the
// event is being replayed and the aggregate takes the version stored in it. An
// event without a version counts as the next one.
//
// A failed ApplyChange leaves the aggregate and the event as they were.
func (b *BaseAggregate) ApplyChangeHelper(ctx context.Context, aggregate AggregateHandler, event *Event, commit bool) error {
	if aggregate == nil {
		return ErrNilAggregate
	}

	if event == nil {
		return ErrNilEvent
	}

	eventVersion := event.GetVersion()
	version := b.GetVersion() + 1

	if !commit && eventVersion != 0 {
		if eventVersion <= b.GetVersion() {
			return fmt.Errorf("%w: event version %d, aggregate version %d", ErrEventOutOfOrder, eventVersion, b.GetVersion())
		}

		version = eventVersion
	}

	if commit {
		event.Version = version
	}

	err := aggregate.ApplyChange(ctx, event)
	if err != nil {
		event.Version = eventVersion

		return err
	}

	b.Version = version

	if commit {
		b.Changes = append(b.GetChanges(), event)
	}

	return nil
}
