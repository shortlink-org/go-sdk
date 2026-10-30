CREATE UNIQUE INDEX CONCURRENTLY "events_transaction_id_global_position_uindex" ON events("transaction_id", "global_position");
