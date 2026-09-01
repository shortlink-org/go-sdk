package outbox

import "embed"

// Migrations holds this package's schema, laid out the way
// postgres/migrate.Migration expects: a "migrations" directory of numbered
// SQL files. It is a source like any other the service applies, and it is the
// only place the outbox table is created — nothing here builds tables at
// start-up behind the migration's back.
//
//	err := migrate.Migration(ctx, store, outbox.Migrations, "outbox")
//
//go:embed migrations/*.sql
var Migrations embed.FS
