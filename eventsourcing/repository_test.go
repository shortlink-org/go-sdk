//go:build unit

package eventsourcing_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/eventsourcing"
	v1 "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	"github.com/shortlink-org/go-sdk/uow"
)

const accountType = "account"

var errBroken = errors.New("broken")

// account is an aggregate without snapshots.
type account struct {
	v1.BaseAggregate

	balance int
}

func (a *account) ApplyChange(_ context.Context, event *v1.Event) error {
	var payload struct {
		Amount int `json:"amount"`
	}

	err := json.Unmarshal([]byte(event.GetPayload()), &payload)
	if err != nil {
		return err
	}

	a.balance += payload.Amount

	return nil
}

func (*account) HandleCommand(context.Context, *v1.BaseCommand) error {
	return nil
}

func (a *account) deposit(t *testing.T, amount int) {
	t.Helper()

	raw, err := json.Marshal(map[string]int{"amount": amount})
	require.NoError(t, err)

	require.NoError(t, a.ApplyChangeHelper(t.Context(), a, &v1.Event{Type: "deposited", Payload: string(raw)}, true))
}

// snapshotAccount is an account with snapshots.
type snapshotAccount struct {
	account

	schemaVersion int32
	restored      bool
	restoreErr    error
}

func (a *snapshotAccount) SnapshotSchemaVersion() int32 {
	return a.schemaVersion
}

func (a *snapshotAccount) MarshalSnapshot() (string, error) {
	raw, err := json.Marshal(map[string]int{"balance": a.balance})

	return string(raw), err
}

func (a *snapshotAccount) RestoreSnapshot(payload string) error {
	if a.restoreErr != nil {
		a.balance = -1

		return a.restoreErr
	}

	var state map[string]int

	err := json.Unmarshal([]byte(payload), &state)
	if err != nil {
		return err
	}

	a.balance = state["balance"]
	a.restored = true

	return nil
}

// memoryStore is an in-memory eventsourcing.Store with its optimistic locking.
type memoryStore struct {
	events    map[string][]*v1.Event
	snapshots map[string]*v1.Snapshot

	snapshotErr error
	saved       int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{events: map[string][]*v1.Event{}, snapshots: map[string]*v1.Snapshot{}}
}

func (s *memoryStore) Append(_ context.Context, aggregateID string, expectedVersion int32, events []*v1.Event) error {
	if int32(len(s.events[aggregateID])) != expectedVersion {
		return eventsourcing.ErrConcurrencyConflict
	}

	for i, event := range events {
		event.AggregateId = aggregateID
		event.Version = expectedVersion + int32(i) + 1
	}

	s.events[aggregateID] = append(s.events[aggregateID], events...)

	return nil
}

func (s *memoryStore) ReadStream(_ context.Context, aggregateID string, afterVersion int32) ([]*v1.Event, error) {
	var events []*v1.Event

	for _, event := range s.events[aggregateID] {
		if event.GetVersion() > afterVersion {
			events = append(events, event)
		}
	}

	return events, nil
}

func (s *memoryStore) LoadSnapshot(_ context.Context, aggregateID string) (*v1.Snapshot, error) {
	if snapshot, ok := s.snapshots[aggregateID]; ok {
		return snapshot, nil
	}

	return &v1.Snapshot{}, nil
}

func (s *memoryStore) SaveSnapshot(_ context.Context, snapshot *v1.Snapshot) error {
	if s.snapshotErr != nil {
		return s.snapshotErr
	}

	s.snapshots[snapshot.GetAggregateId()] = snapshot
	s.saved++

	return nil
}

func newAccountRepository(t *testing.T, store eventsourcing.Store, opts ...eventsourcing.RepositoryOption) *eventsourcing.Repository[*account] {
	t.Helper()

	repository, err := eventsourcing.NewRepository(store, accountType, func() *account { return &account{} }, opts...)
	require.NoError(t, err)

	return repository
}

func newSnapshotRepository(t *testing.T, store eventsourcing.Store, schemaVersion int32, opts ...eventsourcing.RepositoryOption) *eventsourcing.Repository[*snapshotAccount] {
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
	repository := newAccountRepository(t, store)

	aggregate := repository.New()
	aggregate.deposit(t, 10)
	aggregate.deposit(t, 5)

	require.NoError(t, repository.Save(t.Context(), aggregate))
	require.Empty(t, aggregate.Uncommitted())

	loaded, err := repository.Load(t.Context(), aggregate.GetId())
	require.NoError(t, err)
	require.Equal(t, 15, loaded.balance)
	require.EqualValues(t, 2, loaded.GetVersion())
	require.Equal(t, accountType, loaded.GetType())

	loaded.deposit(t, 1)
	require.NoError(t, repository.Save(t.Context(), loaded))

	require.NoError(t, repository.Save(t.Context(), loaded), "nothing to save")
	require.Len(t, store.events[aggregate.GetId()], 3)
	require.Equal(t, accountType, store.events[aggregate.GetId()][2].GetAggregateType())
}

func TestRepositoryLoadNotFound(t *testing.T) {
	repository := newAccountRepository(t, newMemoryStore())

	_, err := repository.Load(t.Context(), uuid.NewV7().String())
	require.ErrorIs(t, err, eventsourcing.ErrAggregateNotFound)
}

func TestRepositoryLoadRejectsAnotherType(t *testing.T) {
	store := newMemoryStore()
	id := uuid.NewV7().String()
	store.events[id] = []*v1.Event{{AggregateId: id, AggregateType: "order", Version: 1, Payload: `{}`}}

	_, err := newAccountRepository(t, store).Load(t.Context(), id)
	require.ErrorIs(t, err, eventsourcing.ErrAggregateTypeMismatch)
}

func TestRepositorySaveConflictKeepsEvents(t *testing.T) {
	repository := newAccountRepository(t, newMemoryStore())

	first := repository.New()
	first.deposit(t, 1)
	require.NoError(t, repository.Save(t.Context(), first))

	stale, err := repository.Load(t.Context(), first.GetId())
	require.NoError(t, err)

	winner, err := repository.Load(t.Context(), first.GetId())
	require.NoError(t, err)

	winner.deposit(t, 1)
	require.NoError(t, repository.Save(t.Context(), winner))

	stale.deposit(t, 1)
	require.ErrorIs(t, repository.Save(t.Context(), stale), eventsourcing.ErrConcurrencyConflict)
	require.Len(t, stale.Uncommitted(), 1)
}

func TestRepositoryUpcasters(t *testing.T) {
	store := newMemoryStore()

	upcast := func(_ context.Context, event *v1.Event) error {
		if event.GetSchemaVersion() == 1 {
			event.Payload = `{"amount": 100}`
		}

		return nil
	}

	id := uuid.NewV7().String()
	store.events[id] = []*v1.Event{{AggregateId: id, AggregateType: accountType, Version: 1, SchemaVersion: 1, Payload: `{"cents": 1}`}}

	loaded, err := newAccountRepository(t, store, eventsourcing.WithUpcasters(upcast)).Load(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, 100, loaded.balance)

	failing := func(context.Context, *v1.Event) error { return errBroken }

	_, err = newAccountRepository(t, store, eventsourcing.WithUpcasters(failing)).Load(t.Context(), id)
	require.ErrorIs(t, err, errBroken)
}

func TestRepositorySnapshotEvery(t *testing.T) {
	store := newMemoryStore()
	repository := newSnapshotRepository(t, store, 1, eventsourcing.WithSnapshotEvery(3))

	aggregate := repository.New()
	aggregate.deposit(t, 1)
	aggregate.deposit(t, 1)
	require.NoError(t, repository.Save(t.Context(), aggregate))
	require.Zero(t, store.saved, "below the policy")

	aggregate.deposit(t, 1)
	aggregate.deposit(t, 1)
	require.NoError(t, repository.Save(t.Context(), aggregate))
	require.Equal(t, 1, store.saved, "the batch crossed version 3")
	require.EqualValues(t, 4, store.snapshots[aggregate.GetId()].GetAggregateVersion())

	loaded, err := repository.Load(t.Context(), aggregate.GetId())
	require.NoError(t, err)
	require.True(t, loaded.restored)
	require.Equal(t, 4, loaded.balance)
	require.EqualValues(t, 4, loaded.GetVersion())
}

func TestRepositorySnapshotFailureDoesNotFailSave(t *testing.T) {
	store := newMemoryStore()
	store.snapshotErr = errBroken
	repository := newSnapshotRepository(t, store, 1, eventsourcing.WithSnapshotEvery(1))

	aggregate := repository.New()
	aggregate.deposit(t, 1)

	require.NoError(t, repository.Save(t.Context(), aggregate))
	require.Empty(t, aggregate.Uncommitted())
}

func TestRepositorySkipsSnapshotInsideTransaction(t *testing.T) {
	store := newMemoryStore()
	repository := newSnapshotRepository(t, store, 1, eventsourcing.WithSnapshotEvery(1))

	aggregate := repository.New()
	aggregate.deposit(t, 1)

	require.NoError(t, repository.Save(uow.WithTx(t.Context(), fakeTx{}), aggregate))
	require.Zero(t, store.saved)
}

func TestRepositoryIgnoresUnusableSnapshot(t *testing.T) {
	t.Run("another schema version", func(t *testing.T) {
		store := newMemoryStore()
		writer := newSnapshotRepository(t, store, 1)

		aggregate := writer.New()
		aggregate.deposit(t, 7)
		require.NoError(t, writer.Save(t.Context(), aggregate))
		require.NoError(t, writer.Snapshot(t.Context(), aggregate.GetId()))

		loaded, err := newSnapshotRepository(t, store, 2).Load(t.Context(), aggregate.GetId())
		require.NoError(t, err)
		require.False(t, loaded.restored)
		require.Equal(t, 7, loaded.balance, "replayed from the events")
	})

	t.Run("a broken payload", func(t *testing.T) {
		store := newMemoryStore()
		writer := newSnapshotRepository(t, store, 1)

		aggregate := writer.New()
		aggregate.deposit(t, 7)
		require.NoError(t, writer.Save(t.Context(), aggregate))
		require.NoError(t, writer.Snapshot(t.Context(), aggregate.GetId()))

		reader, err := eventsourcing.NewRepository(store, accountType, func() *snapshotAccount {
			return &snapshotAccount{schemaVersion: 1, restoreErr: errBroken}
		})
		require.NoError(t, err)

		loaded, err := reader.Load(t.Context(), aggregate.GetId())
		require.NoError(t, err)
		require.Equal(t, 7, loaded.balance, "a fresh aggregate, without the half-restored state")
	})
}

func TestRepositorySnapshotRequiresSnapshotter(t *testing.T) {
	repository := newAccountRepository(t, newMemoryStore())

	aggregate := repository.New()
	aggregate.deposit(t, 1)
	require.NoError(t, repository.Save(t.Context(), aggregate))

	require.ErrorIs(t, repository.Snapshot(t.Context(), aggregate.GetId()), eventsourcing.ErrNotSnapshotter)
}
