//go:build unit

package es_postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
)

const testUUID = "0199f1c2-7a3b-7c10-9e21-5d6f7a8b9c0d"

func TestValidateEvents(t *testing.T) {
	tests := []struct {
		name  string
		event *eventsourcing.Event
		want  error
	}{
		{name: "valid", event: &eventsourcing.Event{AggregateId: testUUID, Version: 1}},
		{name: "valid with an id", event: &eventsourcing.Event{AggregateId: testUUID, Id: testUUID, Version: 2}},
		{name: "nil", event: nil, want: ErrInvalidEvent},
		{name: "version zero", event: &eventsourcing.Event{AggregateId: testUUID}, want: ErrInvalidEvent},
		{name: "aggregate id", event: &eventsourcing.Event{AggregateId: "order-1", Version: 1}, want: ErrInvalidAggregateID},
		{name: "event id", event: &eventsourcing.Event{AggregateId: testUUID, Id: "event-1", Version: 1}, want: ErrInvalidEventID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEvents([]*eventsourcing.Event{tt.event})
			if tt.want == nil {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestConcurrencyConflictError(t *testing.T) {
	cause := errors.New("duplicate key")

	var err error = fmt.Errorf("save: %w", &ConcurrencyConflictError{Err: cause, AggregateID: testUUID, Version: 2})

	require.ErrorIs(t, err, ErrConcurrencyConflict)
	require.ErrorIs(t, err, cause)

	var deprecated *IncorrectUpdatedBillingError

	require.ErrorAs(t, err, &deprecated, "the former name must keep matching")
	require.EqualValues(t, 2, deprecated.Version)
}

func TestOptions(t *testing.T) {
	config := options{snapshotBatchSize: defaultSnapshotBatchSize, snapshotThreshold: defaultSnapshotThreshold}

	for _, opt := range []Option{WithSnapshotBatchSize(0), WithSnapshotThreshold(-1)} {
		opt(&config)
	}

	require.EqualValues(t, defaultSnapshotBatchSize, config.snapshotBatchSize, "a meaningless size keeps the default")
	require.EqualValues(t, defaultSnapshotThreshold, config.snapshotThreshold, "a meaningless threshold keeps the default")

	for _, opt := range []Option{WithSnapshotBatchSize(5), WithSnapshotThreshold(10), WithoutMigrations()} {
		opt(&config)
	}

	require.EqualValues(t, 5, config.snapshotBatchSize)
	require.EqualValues(t, 10, config.snapshotThreshold)
	require.True(t, config.skipMigrations)
}
