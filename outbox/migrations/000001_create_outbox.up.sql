CREATE TABLE IF NOT EXISTS outbox
(
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid         TEXT        NOT NULL,
    topic        TEXT        NOT NULL,
    payload      BYTEA       NOT NULL,
    metadata     JSONB       NOT NULL DEFAULT '{}'::JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ
);

-- The relay only ever reads the undelivered rows of one topic, oldest first.
-- A partial index keeps it the size of the backlog rather than the size of the
-- history.
CREATE INDEX IF NOT EXISTS outbox_pending_idx
    ON outbox (topic, id)
    WHERE delivered_at IS NULL;

-- The reaper only ever reads delivered rows, oldest first.
CREATE INDEX IF NOT EXISTS outbox_delivered_idx
    ON outbox (delivered_at)
    WHERE delivered_at IS NOT NULL;
