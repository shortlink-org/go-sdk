COMMENT ON COLUMN events."aggregate_id" IS 'The aggregate this event belongs to. Unique together with version.';
COMMENT ON COLUMN events."payload" IS 'The body of the event as JSON, its meaning is defined by the type column. Example: {"amount": 5000}';
