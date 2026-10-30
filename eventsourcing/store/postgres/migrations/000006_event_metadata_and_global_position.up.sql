-- Rewrites events under an exclusive lock: global_position and transaction_id give every existing row its own value.
ALTER TABLE events
    ADD COLUMN "schema_version" INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN "correlation_id" TEXT NOT NULL DEFAULT '',
    ADD COLUMN "causation_id" TEXT NOT NULL DEFAULT '',
    ADD COLUMN "metadata" jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN "global_position" BIGINT GENERATED ALWAYS AS IDENTITY,
    ADD COLUMN "transaction_id" xid8 NOT NULL DEFAULT pg_current_xact_id();

COMMENT ON COLUMN events."schema_version" IS 'The version of the payload schema, what an upcaster keys on.';
COMMENT ON COLUMN events."correlation_id" IS 'The id shared by everything one request caused.';
COMMENT ON COLUMN events."causation_id" IS 'The id of the command or event that caused this event.';
COMMENT ON COLUMN events."metadata" IS 'Free-form string pairs, among them the W3C trace context.';
COMMENT ON COLUMN events."global_position" IS 'Unique, but commits may become visible out of this order. Read the global stream by (transaction_id, global_position).';
COMMENT ON COLUMN events."transaction_id" IS 'The transaction that wrote the event. Readers stop at pg_snapshot_xmin so that no event commits behind them.';

ALTER TABLE snapshots
    ADD COLUMN "schema_version" INTEGER NOT NULL DEFAULT 1;

COMMENT ON COLUMN snapshots."schema_version" IS 'The version of the payload schema. A snapshot of another version is ignored and the events are replayed.';
