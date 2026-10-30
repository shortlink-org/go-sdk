//go:build unit

package eventsourcing_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/eventsourcing"
	"github.com/shortlink-org/go-sdk/uow"
)

const accountType = "account"

var errBroken = errors.New("broken")

type deposited struct {
	Amount int `json:"amount"`
}

// account is an aggregate without snapshots.
type account struct {
	eventsourcing.Root

	balance int
	reject  bool
	seen    int64
}

func (a *account) Apply(event *eventsourcing.Event) error {
	a.seen = event.Version

	if a.reject {
		return errBroken
	}

	var payload deposited

	err := json.Unmarshal(event.Payload, &payload)
	if err != nil {
		return err
	}

	a.balance += payload.Amount

	return nil
}

func deposit(t *testing.T, aggregate eventsourcing.Aggregate, amount int) {
	t.Helper()

	event, err := eventsourcing.NewEvent("deposited", deposited{Amount: amount})
	require.NoError(t, err)
	require.NoError(t, eventsourcing.Raise(aggregate, event))
}

func TestNewEventRejectsPayload(t *testing.T) {
	_, err := eventsourcing.NewEvent("broken", make(chan int))
	require.ErrorIs(t, err, eventsourcing.ErrInvalidEvent)
}

func TestRaise(t *testing.T) {
	aggregate := &account{}

	deposit(t, aggregate, 1)
	require.EqualValues(t, 1, aggregate.seen, "Apply must see the new version")
	deposit(t, aggregate, 2)

	require.EqualValues(t, 2, aggregate.Version())
	require.Equal(t, 3, aggregate.balance)
	require.Len(t, aggregate.Uncommitted(), 2)
	require.EqualValues(t, 2, aggregate.Uncommitted()[1].Version)
}

func TestRaiseFailureLeavesStateUntouched(t *testing.T) {
	aggregate := &account{reject: true}

	event, err := eventsourcing.NewEvent("deposited", deposited{Amount: 1})
	require.NoError(t, err)
	require.ErrorIs(t, eventsourcing.Raise(aggregate, event), errBroken)

	require.Zero(t, aggregate.Version())
	require.Empty(t, aggregate.Uncommitted())

	aggregate.reject = false

	deposit(t, aggregate, 1)
	require.EqualValues(t, 1, aggregate.Version(), "a rejected event must not consume a version")
}

func TestRaiseNilArguments(t *testing.T) {
	require.ErrorIs(t, eventsourcing.Raise(nil, &eventsourcing.Event{}), eventsourcing.ErrNilAggregate)
	require.ErrorIs(t, eventsourcing.Raise(&account{}, nil), eventsourcing.ErrInvalidEvent)
}

// snapshotAccount is an account with snapshots.
type snapshotAccount struct {
	account

	schemaVersion int
	restored      bool
	restoreErr    error
}

func (a *snapshotAccount) SnapshotSchemaVersion() int {
	return a.schemaVersion
}

func (a *snapshotAccount) MarshalSnapshot() (json.RawMessage, error) {
	return json.Marshal(map[string]int{"balance": a.balance})
}

func (a *snapshotAccount) RestoreSnapshot(payload json.RawMessage) error {
	if a.restoreErr != nil {
		a.balance = -1

		return a.restoreErr
	}

	var state map[string]int

	err := json.Unmarshal(payload, &state)
	if err != nil {
		return err
	}

	a.balance = state["balance"]
	a.restored = true

	return nil
}

// memoryStore is an in-memory eventsourcing.Store with its optimistic locking.
type memoryStore struct {
	events    map[string][]*eventsourcing.Event
	snapshots map[string]*eventsourcing.Snapshot

	snapshotErr error
	saved       int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{events: map[string][]*eventsourcing.Event{}, snapshots: map[string]*eventsourcing.Snapshot{}}
}

func (s *memoryStore) Append(_ context.Context, aggregateID string, expectedVersion int64, events []*eventsourcing.Event) error {
	if int64(len(s.events[aggregateID])) != expectedVersion {
		return eventsourcing.ErrConcurrencyConflict
	}

	s.events[aggregateID] = append(s.events[aggregateID], events...)

	return nil
}

func (s *memoryStore) ReadStream(_ context.Context, aggregateID string, afterVersion int64) ([]*eventsourcing.Event, error) {
	var events []*eventsourcing.Event

	for _, event := range s.events[aggregateID] {
		if event.Version > afterVersion {
			events = append(events, event)
		}
	}

	return events, nil
}

func (s *memoryStore) LoadSnapshot(_ context.Context, aggregateID string) (*eventsourcing.Snapshot, error) {
	snapshot, ok := s.snapshots[aggregateID]
	if !ok {
		return nil, eventsourcing.ErrSnapshotNotFound
	}

	return snapshot, nil
}

func (s *memoryStore) SaveSnapshot(_ context.Context, snapshot *eventsourcing.Snapshot) error {
	if s.snapshotErr != nil {
		return s.snapshotErr
	}

	s.snapshots[snapshot.AggregateID] = snapshot
	s.saved++

	return nil
}

func newAccounts(t *testing.T, store eventsourcing.Store, opts ...eventsourcing.RepositoryOption) *eventsourcing.Repository[*account] {
	t.Helper()

	repository, err := eventsourcing.NewRepository(store, accountType, func() *account { return &account{} }, opts...)
	require.NoError(t, err)

	return repository
}

func newSnapshotAccounts(t *testing.T, store eventsourcing.Store, schemaVersion int, opts ...eventsourcing.RepositoryOption) *eventsourcing.Repository[*snapshotAccount] {
	t.Helper()

	repository, err := eventsourcing.NewRepository(store, accountType, func() *snapshotAccount {
		return &snapshotAccount{schemaVersion: schemaVersion}
	}, opts...)
	require.NoError(t, err)

	return repository
}

func TestNewRepositoryValidates(t *testing.T) {
	factory := func() *account { return &account{} }

	_, err := eventsourcing.NewRepository(nil, accountType, factory)
	require.ErrorIs(t, err, eventsourcing.ErrNilStore)

	_, err = eventsourcing.NewRepository[*account](newMemoryStore(), accountType, nil)
	require.ErrorIs(t, err, eventsourcing.ErrNilFactory)

	_, err = eventsourcing.NewRepository(newMemoryStore(), "", factory)
	require.ErrorIs(t, err, eventsourcing.ErrEmptyAggregateType)
}

func TestRepositorySaveAndLoad(t *testing.T) {
	store := newMemoryStore()
	repository := newAccounts(t, store)

	aggregate := repository.New()
	deposit(t, aggregate, 10)
	deposit(t, aggregate, 5)

	require.NoError(t, repository.Save(t.Context(), aggregate))
	require.Empty(t, aggregate.Uncommitted())

	loaded, err := repository.Load(t.Context(), aggregate.ID())
	require.NoError(t, err)
	require.Equal(t, 15, loaded.balance)
	require.EqualValues(t, 2, loaded.Version())
	require.Equal(t, accountType, loaded.Type())

	deposit(t, loaded, 1)
	require.NoError(t, repository.Save(t.Context(), loaded))
	require.NoError(t, repository.Save(t.Context(), loaded), "nothing to save")

	stored := store.events[aggregate.ID()]
	require.Len(t, stored, 3)
	require.Equal(t, aggregate.ID(), stored[2].AggregateID)
	require.Equal(t, accountType, stored[2].AggregateType)
}

func TestRepositorySaveAssignsIdentity(t *testing.T) {
	store := newMemoryStore()
	repository := newAccounts(t, store)

	aggregate := &account{}
	deposit(t, aggregate, 1)

	require.NoError(t, repository.Save(t.Context(), aggregate))
	require.NotEmpty(t, aggregate.ID())
	require.Equal(t, accountType, aggregate.Type())
	require.Equal(t, aggregate.ID(), store.events[aggregate.ID()][0].AggregateID)
}

func TestRepositoryLoadNotFound(t *testing.T) {
	_, err := newAccounts(t, newMemoryStore()).Load(t.Context(), uuid.NewV7().String())
	require.ErrorIs(t, err, eventsourcing.ErrAggregateNotFound)
}

func TestRepositoryLoadRejectsAnotherType(t *testing.T) {
	store := newMemoryStore()
	id := uuid.NewV7().String()
	store.events[id] = []*eventsourcing.Event{{AggregateID: id, AggregateType: "order", Version: 1, Payload: json.RawMessage(`{}`)}}

	_, err := newAccounts(t, store).Load(t.Context(), id)
	require.ErrorIs(t, err, eventsourcing.ErrAggregateTypeMismatch)
}

func TestRepositoryLoadRejectsEventOutOfOrder(t *testing.T) {
	store := newMemoryStore()
	id := uuid.NewV7().String()
	store.events[id] = []*eventsourcing.Event{
		{AggregateID: id, AggregateType: accountType, Version: 2, Payload: json.RawMessage(`{}`)},
		{AggregateID: id, AggregateType: accountType, Version: 2, Payload: json.RawMessage(`{}`)},
	}

	_, err := newAccounts(t, store).Load(t.Context(), id)
	require.ErrorIs(t, err, eventsourcing.ErrEventOutOfOrder)
}

func TestRepositorySaveConflictKeepsEvents(t *testing.T) {
	repository := newAccounts(t, newMemoryStore())

	first := repository.New()
	deposit(t, first, 1)
	require.NoError(t, repository.Save(t.Context(), first))

	stale, err := repository.Load(t.Context(), first.ID())
	require.NoError(t, err)

	winner, err := repository.Load(t.Context(), first.ID())
	require.NoError(t, err)

	deposit(t, winner, 1)
	require.NoError(t, repository.Save(t.Context(), winner))

	deposit(t, stale, 1)
	require.ErrorIs(t, repository.Save(t.Context(), stale), eventsourcing.ErrConcurrencyConflict)
	require.Len(t, stale.Uncommitted(), 1)
}

func TestRepositoryUpcasters(t *testing.T) {
	store := newMemoryStore()
	id := uuid.NewV7().String()
	store.events[id] = []*eventsourcing.Event{
		{AggregateID: id, AggregateType: accountType, Version: 1, SchemaVersion: 1, Payload: json.RawMessage(`{"cents": 1}`)},
	}

	upcast := func(event *eventsourcing.Event) error {
		if event.SchemaVersion == 1 {
			event.Payload = json.RawMessage(`{"amount": 100}`)
		}

		return nil
	}

	loaded, err := newAccounts(t, store, eventsourcing.WithUpcasters(upcast)).Load(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, 100, loaded.balance)

	failing := func(*eventsourcing.Event) error { return errBroken }

	_, err = newAccounts(t, store, eventsourcing.WithUpcasters(failing)).Load(t.Context(), id)
	require.ErrorIs(t, err, errBroken)
}

func TestRepositorySnapshotEvery(t *testing.T) {
	store := newMemoryStore()
	repository := newSnapshotAccounts(t, store, 1, eventsourcing.WithSnapshotEvery(3))

	aggregate := repository.New()
	deposit(t, aggregate, 1)
	deposit(t, aggregate, 1)
	require.NoError(t, repository.Save(t.Context(), aggregate))
	require.Zero(t, store.saved, "below the policy")

	deposit(t, aggregate, 1)
	deposit(t, aggregate, 1)
	require.NoError(t, repository.Save(t.Context(), aggregate))
	require.Equal(t, 1, store.saved, "the batch crossed version 3")
	require.EqualValues(t, 4, store.snapshots[aggregate.ID()].Version)

	loaded, err := repository.Load(t.Context(), aggregate.ID())
	require.NoError(t, err)
	require.True(t, loaded.restored)
	require.Equal(t, 4, loaded.balance)
	require.EqualValues(t, 4, loaded.Version())
}

func TestRepositorySnapshotFailureDoesNotFailSave(t *testing.T) {
	store := newMemoryStore()
	store.snapshotErr = errBroken
	repository := newSnapshotAccounts(t, store, 1, eventsourcing.WithSnapshotEvery(1))

	aggregate := repository.New()
	deposit(t, aggregate, 1)

	require.NoError(t, repository.Save(t.Context(), aggregate))
	require.Empty(t, aggregate.Uncommitted())
}

// fakeTx only marks a context as carrying a transaction.
type fakeTx struct {
	pgx.Tx
}

func TestRepositorySkipsSnapshotInsideTransaction(t *testing.T) {
	store := newMemoryStore()
	repository := newSnapshotAccounts(t, store, 1, eventsourcing.WithSnapshotEvery(1))

	aggregate := repository.New()
	deposit(t, aggregate, 1)

	require.NoError(t, repository.Save(uow.WithTx(t.Context(), fakeTx{}), aggregate))
	require.Zero(t, store.saved)
}

func TestRepositoryIgnoresUnusableSnapshot(t *testing.T) {
	t.Run("another schema version", func(t *testing.T) {
		store := newMemoryStore()
		writer := newSnapshotAccounts(t, store, 1)

		aggregate := writer.New()
		deposit(t, aggregate, 7)
		require.NoError(t, writer.Save(t.Context(), aggregate))
		require.NoError(t, writer.Snapshot(t.Context(), aggregate.ID()))

		loaded, err := newSnapshotAccounts(t, store, 2).Load(t.Context(), aggregate.ID())
		require.NoError(t, err)
		require.False(t, loaded.restored)
		require.Equal(t, 7, loaded.balance, "replayed from the events")
	})

	t.Run("a payload that does not restore", func(t *testing.T) {
		store := newMemoryStore()
		writer := newSnapshotAccounts(t, store, 1)

		aggregate := writer.New()
		deposit(t, aggregate, 7)
		require.NoError(t, writer.Save(t.Context(), aggregate))
		require.NoError(t, writer.Snapshot(t.Context(), aggregate.ID()))

		reader, err := eventsourcing.NewRepository(store, accountType, func() *snapshotAccount {
			return &snapshotAccount{schemaVersion: 1, restoreErr: errBroken}
		})
		require.NoError(t, err)

		loaded, err := reader.Load(t.Context(), aggregate.ID())
		require.NoError(t, err)
		require.Equal(t, 7, loaded.balance, "a fresh aggregate, without the half-restored state")
	})
}

func TestRepositorySnapshotRequiresSnapshotter(t *testing.T) {
	repository := newAccounts(t, newMemoryStore())

	aggregate := repository.New()
	deposit(t, aggregate, 1)
	require.NoError(t, repository.Save(t.Context(), aggregate))

	require.ErrorIs(t, repository.Snapshot(t.Context(), aggregate.ID()), eventsourcing.ErrNotSnapshotter)
}
