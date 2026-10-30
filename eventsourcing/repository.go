package eventsourcing

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
	"github.com/shortlink-org/go-sdk/uow"
)

const instrumentationName = "github.com/shortlink-org/go-sdk/eventsourcing"

var (
	// ErrNilStore is returned by NewRepository without a store.
	ErrNilStore = errors.New("eventsourcing: repository requires a store")

	// ErrNilFactory is returned by NewRepository without a factory.
	ErrNilFactory = errors.New("eventsourcing: repository requires a factory")

	// ErrEmptyAggregateType is returned by NewRepository without an aggregate type.
	ErrEmptyAggregateType = errors.New("eventsourcing: repository requires an aggregate type")

	// ErrAggregateNotFound is returned by Repository.Load for an aggregate without
	// a snapshot or events.
	ErrAggregateNotFound = errors.New("eventsourcing: aggregate not found")

	// ErrAggregateTypeMismatch is returned by Repository.Load for an aggregate of
	// another type than the repository handles.
	ErrAggregateTypeMismatch = errors.New("eventsourcing: aggregate type mismatch")

	// ErrNotSnapshotter is returned by Repository.Snapshot for an aggregate that
	// does not implement Snapshotter.
	ErrNotSnapshotter = errors.New("eventsourcing: aggregate does not implement Snapshotter")
)

// Aggregate is what Repository needs from an aggregate. A pointer to a type that
// embeds BaseAggregate and implements ApplyChange and HandleCommand satisfies it.
type Aggregate interface {
	eventsourcing.AggregateHandler
	Base() *eventsourcing.BaseAggregate
}

// Snapshotter is implemented by an aggregate that can be restored from a snapshot.
// Implementing it is optional: without it, Load replays all the events.
type Snapshotter interface {
	// SnapshotSchemaVersion is the version of the MarshalSnapshot format. Bump it
	// when the format changes: snapshots of another version are ignored and the
	// events are replayed.
	SnapshotSchemaVersion() int32
	// MarshalSnapshot returns the state of the aggregate as JSON.
	MarshalSnapshot() (string, error)
	// RestoreSnapshot sets the state of the aggregate from MarshalSnapshot's output.
	RestoreSnapshot(payload string) error
}

// Upcaster brings an event of an older schema up to date in place, before the
// aggregate sees it. It leaves events it does not recognize alone.
type Upcaster func(ctx context.Context, event *eventsourcing.Event) error

type repositoryOptions struct {
	snapshotEvery int32
	upcasters     []Upcaster
}

// RepositoryOption configures a Repository.
type RepositoryOption func(*repositoryOptions)

// WithSnapshotEvery makes Repository.Save take a snapshot whenever the version of
// the aggregate crosses a multiple of events. Off by default.
//
// The events are saved by then, so a failed snapshot is recorded on the span and
// does not fail Save. Inside a uow transaction no snapshot is taken: an error would
// abort the transaction of the caller. Use Repository.Snapshot from a background
// job there, driven by GetAggregateWithoutSnapshot.
func WithSnapshotEvery(events int32) RepositoryOption {
	return func(o *repositoryOptions) {
		if events > 0 {
			o.snapshotEvery = events
		}
	}
}

// WithUpcasters sets the upcasters Load runs on each event, in the given order.
func WithUpcasters(upcasters ...Upcaster) RepositoryOption {
	return func(o *repositoryOptions) {
		o.upcasters = append(o.upcasters, upcasters...)
	}
}

// Repository loads and saves aggregates of one type.
type Repository[T Aggregate] struct {
	store         Store
	aggregateType string
	factory       func() T
	opts          repositoryOptions
}

// NewRepository returns a Repository. The factory returns an empty aggregate.
func NewRepository[T Aggregate](store Store, aggregateType string, factory func() T, opts ...RepositoryOption) (*Repository[T], error) {
	if store == nil {
		return nil, ErrNilStore
	}

	if factory == nil {
		return nil, ErrNilFactory
	}

	if aggregateType == "" {
		return nil, ErrEmptyAggregateType
	}

	repository := &Repository[T]{store: store, aggregateType: aggregateType, factory: factory}

	for _, opt := range opts {
		opt(&repository.opts)
	}

	return repository, nil
}

// New returns an empty aggregate with a new id.
//
//nolint:ireturn // T is the caller's own aggregate type
func (r *Repository[T]) New() T {
	return r.empty(uuid.NewV7().String())
}

// Load rebuilds the aggregate from its snapshot, if it has a usable one, and the
// events after it. It returns an error matching ErrAggregateNotFound for an
// aggregate that does not exist.
//
//nolint:ireturn // T is the caller's own aggregate type
func (r *Repository[T]) Load(ctx context.Context, aggregateID string) (T, error) {
	ctx, span := tracer().Start(ctx, "Repository.Load")
	defer span.End()

	span.SetAttributes(semconv.AggregateID(aggregateID), semconv.AggregateType(r.aggregateType))

	var zero T

	snapshot, err := r.store.LoadSnapshot(ctx, aggregateID)
	if err != nil {
		return zero, recordError(span, err)
	}

	aggregate := r.restore(span, aggregateID, snapshot)

	events, err := r.store.ReadStream(ctx, aggregateID, aggregate.Base().GetVersion())
	if err != nil {
		return zero, recordError(span, err)
	}

	if aggregate.Base().GetVersion() == 0 && len(events) == 0 {
		return zero, recordError(span, fmt.Errorf("%w: %s", ErrAggregateNotFound, aggregateID))
	}

	for _, event := range events {
		err = r.replay(ctx, aggregate, event)
		if err != nil {
			return zero, recordError(span, err)
		}
	}

	return aggregate, nil
}

// Save appends the uncommitted events of the aggregate and clears them. It returns
// an error matching ErrConcurrencyConflict when another writer got there first, and
// then keeps the events: load the aggregate again and retry the command.
//
// Inside a uow transaction, the events are cleared before the caller commits. After
// a rollback, load the aggregate again.
func (r *Repository[T]) Save(ctx context.Context, aggregate T) error {
	ctx, span := tracer().Start(ctx, "Repository.Save")
	defer span.End()

	base := aggregate.Base()
	events := aggregate.Uncommitted()

	if len(events) == 0 {
		return nil
	}

	if base.GetId() == "" {
		base.Id = uuid.NewV7().String()
	}

	if base.GetType() == "" {
		base.Type = r.aggregateType
	}

	span.SetAttributes(semconv.AggregateID(base.GetId()), semconv.AggregateType(base.GetType()))

	for _, event := range events {
		if event.GetAggregateType() == "" {
			event.AggregateType = base.GetType()
		}
	}

	expectedVersion := base.GetVersion() - int32(len(events)) //nolint:gosec // an aggregate's events fit its int32 version

	err := r.store.Append(ctx, base.GetId(), expectedVersion, events)
	if err != nil {
		return recordError(span, err)
	}

	aggregate.ClearUncommitted()

	if r.snapshotDue(ctx, expectedVersion, base.GetVersion()) {
		err = r.saveSnapshot(ctx, aggregate)
		if err != nil && !errors.Is(err, ErrNotSnapshotter) {
			span.RecordError(err)
		}
	}

	return nil
}

// Snapshot saves a snapshot of the current state of the aggregate.
func (r *Repository[T]) Snapshot(ctx context.Context, aggregateID string) error {
	ctx, span := tracer().Start(ctx, "Repository.Snapshot")
	defer span.End()

	aggregate, err := r.Load(ctx, aggregateID)
	if err != nil {
		return recordError(span, err)
	}

	err = r.saveSnapshot(ctx, aggregate)
	if err != nil {
		return recordError(span, err)
	}

	return nil
}

//nolint:ireturn // T is the caller's own aggregate type
func (r *Repository[T]) empty(aggregateID string) T {
	aggregate := r.factory()
	base := aggregate.Base()
	base.Id = aggregateID
	base.Type = r.aggregateType

	return aggregate
}

// restore returns the aggregate at the snapshot, or an empty one when the snapshot
// is missing or not usable. A snapshot can always be rebuilt from the events, so a
// broken one is no reason to fail the load.
//
//nolint:ireturn // T is the caller's own aggregate type
func (r *Repository[T]) restore(span trace.Span, aggregateID string, snapshot *eventsourcing.Snapshot) T {
	aggregate := r.empty(aggregateID)

	snapshotter, ok := any(aggregate).(Snapshotter)
	if !ok || snapshot.GetAggregateVersion() == 0 || snapshot.GetAggregateType() != r.aggregateType {
		return aggregate
	}

	if normalizeSchemaVersion(snapshot.GetSchemaVersion()) != normalizeSchemaVersion(snapshotter.SnapshotSchemaVersion()) {
		return aggregate
	}

	err := snapshotter.RestoreSnapshot(snapshot.GetPayload())
	if err != nil {
		span.RecordError(err)

		return r.empty(aggregateID)
	}

	aggregate.Base().Version = snapshot.GetAggregateVersion()

	return aggregate
}

func (r *Repository[T]) replay(ctx context.Context, aggregate T, event *eventsourcing.Event) error {
	if event.GetAggregateType() != r.aggregateType {
		return fmt.Errorf("%w: %s is a %s, not a %s", ErrAggregateTypeMismatch, event.GetAggregateId(), event.GetAggregateType(), r.aggregateType)
	}

	for _, upcaster := range r.opts.upcasters {
		err := upcaster(ctx, event)
		if err != nil {
			return err
		}
	}

	return aggregate.ApplyChangeHelper(ctx, aggregate, event, false)
}

func (r *Repository[T]) snapshotDue(ctx context.Context, before, after int32) bool {
	every := r.opts.snapshotEvery

	return every > 0 && after/every > before/every && !uow.HasTx(ctx)
}

func (r *Repository[T]) saveSnapshot(ctx context.Context, aggregate T) error {
	snapshotter, ok := any(aggregate).(Snapshotter)
	if !ok {
		return ErrNotSnapshotter
	}

	payload, err := snapshotter.MarshalSnapshot()
	if err != nil {
		return err
	}

	base := aggregate.Base()

	return r.store.SaveSnapshot(ctx, &eventsourcing.Snapshot{
		AggregateId:      base.GetId(),
		AggregateType:    base.GetType(),
		AggregateVersion: base.GetVersion(),
		SchemaVersion:    normalizeSchemaVersion(snapshotter.SnapshotSchemaVersion()),
		Payload:          payload,
	})
}

// normalizeSchemaVersion treats an unset schema version as 1, as the store does.
func normalizeSchemaVersion(version int32) int32 {
	if version == 0 {
		return 1
	}

	return version
}

func tracer() trace.Tracer { //nolint:ireturn // the OpenTelemetry API returns the interface
	return otel.Tracer(instrumentationName)
}

func recordError(span trace.Span, err error) error {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())

	return err
}
