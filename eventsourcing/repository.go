package eventsourcing

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
	"github.com/shortlink-org/go-sdk/uow"
)

const instrumentationName = "github.com/shortlink-org/go-sdk/eventsourcing"

// Upcaster brings an event of an older schema up to date in place, before the
// aggregate sees it. It leaves events it does not recognize alone.
type Upcaster func(event *Event) error

// RepositoryOption configures a Repository.
type RepositoryOption func(*repositoryOptions)

type repositoryOptions struct {
	snapshotEvery int64
	upcasters     []Upcaster
}

// WithSnapshotEvery makes Repository.Save take a snapshot whenever the version of
// the aggregate crosses a multiple of events. Off by default.
//
// The events are saved by then, so a failed snapshot is recorded on the span and
// does not fail Save. Inside a uow transaction no snapshot is taken: an error would
// abort the transaction of the caller. Use Repository.Snapshot from a background
// job there, driven by SnapshotScheduler.
func WithSnapshotEvery(events int64) RepositoryOption {
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
	return r.NewWithID(uuid.NewV7().String())
}

// NewWithID returns an empty aggregate with the given id, which must be a UUID.
//
//nolint:ireturn // T is the caller's own aggregate type
func (r *Repository[T]) NewWithID(aggregateID string) T {
	aggregate := r.factory()
	root := aggregate.root()
	root.id = aggregateID
	root.aggregateType = r.aggregateType

	return aggregate
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

	aggregate, err := r.restore(ctx, span, aggregateID)
	if err != nil {
		return zero, recordError(span, err)
	}

	root := aggregate.root()

	events, err := r.store.ReadStream(ctx, aggregateID, root.version)
	if err != nil {
		return zero, recordError(span, err)
	}

	if root.version == 0 && len(events) == 0 {
		return zero, recordError(span, fmt.Errorf("%w: %s", ErrAggregateNotFound, aggregateID))
	}

	for _, event := range events {
		err = r.replay(aggregate, event)
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

	root := aggregate.root()
	events := root.uncommitted

	if len(events) == 0 {
		return nil
	}

	if root.id == "" {
		root.id = uuid.NewV7().String()
	}

	root.aggregateType = r.aggregateType

	span.SetAttributes(semconv.AggregateID(root.id), semconv.AggregateType(root.aggregateType))

	for _, event := range events {
		event.AggregateID = root.id
		event.AggregateType = root.aggregateType
	}

	expectedVersion := root.version - int64(len(events))

	err := r.store.Append(ctx, root.id, expectedVersion, events)
	if err != nil {
		return recordError(span, err)
	}

	root.uncommitted = nil

	if r.snapshotDue(ctx, expectedVersion, root.version) {
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

// restore returns the aggregate at its snapshot, or an empty one when there is no
// usable snapshot. A snapshot can always be rebuilt from the events, so one that
// does not restore is no reason to fail the load.
//
//nolint:ireturn // T is the caller's own aggregate type
func (r *Repository[T]) restore(ctx context.Context, span trace.Span, aggregateID string) (T, error) {
	aggregate := r.NewWithID(aggregateID)

	snapshotter, ok := any(aggregate).(Snapshotter)
	if !ok {
		return aggregate, nil
	}

	snapshot, err := r.store.LoadSnapshot(ctx, aggregateID)
	if errors.Is(err, ErrSnapshotNotFound) {
		return aggregate, nil
	}

	if err != nil {
		return aggregate, err
	}

	if snapshot.AggregateType != r.aggregateType {
		return aggregate, nil
	}

	if normalizeSchemaVersion(snapshot.SchemaVersion) != normalizeSchemaVersion(snapshotter.SnapshotSchemaVersion()) {
		return aggregate, nil
	}

	err = snapshotter.RestoreSnapshot(snapshot.Payload)
	if err != nil {
		span.RecordError(err)

		return r.NewWithID(aggregateID), nil
	}

	aggregate.root().version = snapshot.Version

	return aggregate, nil
}

func (r *Repository[T]) replay(aggregate T, event *Event) error {
	if event.AggregateType != r.aggregateType {
		return fmt.Errorf("%w: %s is a %s, not a %s", ErrAggregateTypeMismatch, event.AggregateID, event.AggregateType, r.aggregateType)
	}

	for _, upcaster := range r.opts.upcasters {
		err := upcaster(event)
		if err != nil {
			return err
		}
	}

	return replay(aggregate, event)
}

func (r *Repository[T]) snapshotDue(ctx context.Context, before, after int64) bool {
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

	root := aggregate.root()

	return r.store.SaveSnapshot(ctx, &Snapshot{
		AggregateID:   root.id,
		AggregateType: root.aggregateType,
		Version:       root.version,
		SchemaVersion: normalizeSchemaVersion(snapshotter.SnapshotSchemaVersion()),
		Payload:       payload,
	})
}

// normalizeSchemaVersion treats an unset schema version as 1, as the store does.
func normalizeSchemaVersion(version int) int {
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
