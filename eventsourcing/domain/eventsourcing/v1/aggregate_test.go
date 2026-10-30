//go:build unit

package v1_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
)

var errRejected = errors.New("rejected")

type account struct {
	v1.BaseAggregate

	reject      bool
	seenVersion int32
}

func (a *account) ApplyChange(_ context.Context, event *v1.Event) error {
	a.seenVersion = event.GetVersion()

	if a.reject {
		return errRejected
	}

	return nil
}

func (*account) HandleCommand(context.Context, *v1.BaseCommand) error {
	return nil
}

func TestApplyChangeHelperCommit(t *testing.T) {
	aggregate := &account{}

	first, second := &v1.Event{}, &v1.Event{}

	require.NoError(t, aggregate.ApplyChangeHelper(t.Context(), aggregate, first, true))
	require.EqualValues(t, 1, aggregate.seenVersion, "ApplyChange must see the new version")
	require.NoError(t, aggregate.ApplyChangeHelper(t.Context(), aggregate, second, true))

	require.EqualValues(t, 1, first.GetVersion())
	require.EqualValues(t, 2, second.GetVersion())
	require.EqualValues(t, 2, aggregate.GetVersion())
	require.Equal(t, []*v1.Event{first, second}, aggregate.Uncommitted())
}

func TestApplyChangeHelperFailureLeavesStateUntouched(t *testing.T) {
	aggregate := &account{reject: true}
	event := &v1.Event{Version: 7}

	err := aggregate.ApplyChangeHelper(t.Context(), aggregate, event, true)
	require.ErrorIs(t, err, errRejected)

	require.EqualValues(t, 0, aggregate.GetVersion())
	require.EqualValues(t, 7, event.GetVersion())
	require.Empty(t, aggregate.Uncommitted())

	aggregate.reject = false

	next := &v1.Event{}
	require.NoError(t, aggregate.ApplyChangeHelper(t.Context(), aggregate, next, true))
	require.EqualValues(t, 1, next.GetVersion(), "a rejected event must not consume a version")
}

func TestApplyChangeHelperReplay(t *testing.T) {
	t.Run("takes the stored version", func(t *testing.T) {
		aggregate := &account{}

		require.NoError(t, aggregate.ApplyChangeHelper(t.Context(), aggregate, &v1.Event{Version: 42}, false))
		require.EqualValues(t, 42, aggregate.GetVersion())
		require.Empty(t, aggregate.Uncommitted())
	})

	t.Run("counts an event without a version as the next one", func(t *testing.T) {
		aggregate := &account{}
		aggregate.Version = 3

		require.NoError(t, aggregate.ApplyChangeHelper(t.Context(), aggregate, &v1.Event{}, false))
		require.EqualValues(t, 4, aggregate.GetVersion())
	})

	t.Run("rejects an event that is not newer", func(t *testing.T) {
		aggregate := &account{}
		aggregate.Version = 5

		err := aggregate.ApplyChangeHelper(t.Context(), aggregate, &v1.Event{Version: 5}, false)
		require.ErrorIs(t, err, v1.ErrEventOutOfOrder)
		require.EqualValues(t, 5, aggregate.GetVersion())
	})
}

func TestApplyChangeHelperNilArguments(t *testing.T) {
	aggregate := &account{}

	require.ErrorIs(t, aggregate.ApplyChangeHelper(t.Context(), aggregate, nil, true), v1.ErrNilEvent)
	require.ErrorIs(t, aggregate.ApplyChangeHelper(t.Context(), nil, &v1.Event{}, true), v1.ErrNilAggregate)
}
