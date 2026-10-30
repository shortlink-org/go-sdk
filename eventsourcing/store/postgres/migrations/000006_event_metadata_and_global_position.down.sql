ALTER TABLE snapshots
    DROP COLUMN "schema_version";

ALTER TABLE events
    DROP COLUMN "transaction_id",
    DROP COLUMN "global_position",
    DROP COLUMN "metadata",
    DROP COLUMN "causation_id",
    DROP COLUMN "correlation_id",
    DROP COLUMN "schema_version";
