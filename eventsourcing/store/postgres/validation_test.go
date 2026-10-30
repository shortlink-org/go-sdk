//go:build unit

package postgres

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/eventsourcing"
)

const testUUID = "0199f1c2-7a3b-7c10-9e21-5d6f7a8b9c0d"

func validEvent() *eventsourcing.Event {
	return &eventsourcing.Event{AggregateType: "account", Type: "deposited", Payload: json.RawMessage(`{}`)}
}

func TestPrepareEvents(t *testing.T) {
	event := validEvent()

	require.NoError(t, prepareEvents(testUUID, 2, []*eventsourcing.Event{event}))
	require.Equal(t, testUUID, event.AggregateID)
	require.EqualValues(t, 3, event.Version)

	require.ErrorIs(t, prepareEvents("order-1", 0, nil), eventsourcing.ErrInvalidAggregateID)
	require.ErrorIs(t, prepareEvents(testUUID, -1, nil), eventsourcing.ErrInvalidEvent)
}

func TestPrepareEventsRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(event *eventsourcing.Event)
		want   error
	}{
		{name: "another aggregate", mutate: func(e *eventsourcing.Event) { e.AggregateID = "other" }, want: eventsourcing.ErrInvalidEvent},
		{name: "another version", mutate: func(e *eventsourcing.Event) { e.Version = 4 }, want: eventsourcing.ErrInvalidEvent},
		{name: "no aggregate type", mutate: func(e *eventsourcing.Event) { e.AggregateType = "" }, want: eventsourcing.ErrInvalidEvent},
		{name: "no type", mutate: func(e *eventsourcing.Event) { e.Type = "" }, want: eventsourcing.ErrInvalidEvent},
		{name: "no payload", mutate: func(e *eventsourcing.Event) { e.Payload = nil }, want: eventsourcing.ErrInvalidEvent},
		{name: "broken payload", mutate: func(e *eventsourcing.Event) { e.Payload = json.RawMessage(`{`) }, want: eventsourcing.ErrInvalidEvent},
		{name: "event id", mutate: func(e *eventsourcing.Event) { e.ID = "event-1" }, want: eventsourcing.ErrInvalidEventID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validEvent()
			tt.mutate(event)

			require.ErrorIs(t, prepareEvents(testUUID, 0, []*eventsourcing.Event{event}), tt.want)
		})
	}

	require.ErrorIs(t, prepareEvents(testUUID, 0, []*eventsourcing.Event{nil}), eventsourcing.ErrInvalidEvent)
}

func TestParseCursor(t *testing.T) {
	transactionID, position, err := parseCursor("")
	require.NoError(t, err)
	require.Equal(t, "0", transactionID)
	require.Zero(t, position)

	transactionID, position, err = parseCursor("812-40")
	require.NoError(t, err)
	require.Equal(t, "812", transactionID)
	require.EqualValues(t, 40, position)

	for _, cursor := range []string{"812", "a-1", "1-b", "1--1", "1;drop-1"} {
		_, _, err = parseCursor(cursor)
		require.ErrorIs(t, err, eventsourcing.ErrInvalidCursor, cursor)
	}
}
