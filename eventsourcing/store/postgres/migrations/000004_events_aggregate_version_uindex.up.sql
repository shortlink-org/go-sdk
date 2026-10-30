CREATE UNIQUE INDEX CONCURRENTLY "events_aggregate_id_version_uindex" ON events("aggregate_id", "version");
