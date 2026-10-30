CREATE SCHEMA IF NOT EXISTS eventsourcing;

CREATE TABLE eventsourcing.aggregates (
    id UUID PRIMARY KEY,
    type TEXT NOT NULL,
    version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE eventsourcing.events (
    id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL REFERENCES eventsourcing.aggregates (id),
    aggregate_type TEXT NOT NULL,
    version BIGINT NOT NULL,
    type TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    payload JSONB NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    global_position BIGINT GENERATED ALWAYS AS IDENTITY,
    transaction_id XID8 NOT NULL DEFAULT pg_current_xact_id(),
    CONSTRAINT events_aggregate_version_key UNIQUE (aggregate_id, version),
    CONSTRAINT events_global_stream_key UNIQUE (transaction_id, global_position)
) WITH (fillfactor = 100);

COMMENT ON TABLE eventsourcing.events IS 'Append-only: rows are never updated.';
COMMENT ON COLUMN eventsourcing.events.global_position IS 'Unique, but commits may become visible out of this order. Read the global stream by (transaction_id, global_position).';
COMMENT ON COLUMN eventsourcing.events.transaction_id IS 'The transaction that wrote the event. Readers stop at pg_snapshot_xmin so that no event commits behind them.';

CREATE TABLE eventsourcing.snapshots (
    aggregate_id UUID PRIMARY KEY REFERENCES eventsourcing.aggregates (id),
    aggregate_type TEXT NOT NULL,
    version BIGINT NOT NULL,
    schema_version INTEGER NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
