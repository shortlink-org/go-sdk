package migrate

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // register pgx driver
	"github.com/johejo/golang-migrate-extra/source/iofs"

	"github.com/shortlink-org/go-sdk/db"
)

// Migration applies migrations from the given filesystem to the database.
// The filesystem should contain a "migrations" directory with migration files.
func Migration(ctx context.Context, store db.DB, fsys fs.FS, tableName string) error {
	client, ok := store.GetConn().(*pgxpool.Pool)
	if !ok {
		return db.ErrGetConnection
	}

	var retErr error

	driverMigrations, err := iofs.New(fsys, "migrations")
	if err != nil {
		retErr = &MigrationError{
			Err:         err,
			Description: "failed to create migration source",
		}
		return retErr
	}

	// Get connection string from pool config
	connStr := client.Config().ConnString()

	// Open separate sql.DB connection for migrations
	conn, err := sql.Open("pgx", connStr)
	if err != nil {
		retErr = &MigrationError{
			Err:         err,
			Description: "failed to open migration connection",
		}
		return retErr
	}
	defer func() {
		retErr = errors.Join(retErr, conn.Close())
	}()

	// Verify connection
	if err := conn.PingContext(ctx); err != nil {
		retErr = &MigrationError{
			Err:         err,
			Description: "failed to ping migration connection",
		}
		return retErr
	}

	driverDB, err := pgx.WithInstance(conn, &pgx.Config{
		MigrationsTable: "schema_migrations_" + strings.ReplaceAll(tableName, "-", "_"),
	})
	if err != nil {
		retErr = &MigrationError{
			Err:         err,
			Description: "failed to create migration driver",
		}
		return retErr
	}

	migration, err := migrate.NewWithInstance("iofs", driverMigrations, "postgres", driverDB)
	if err != nil {
		retErr = &MigrationError{
			Err:         err,
			Description: "failed to create migration instance",
		}
		return retErr
	}

	defer func() {
		sourceErr, dbErr := migration.Close()
		retErr = errors.Join(retErr, sourceErr, dbErr)
	}()

	if err := migration.Up(); err != nil && err != migrate.ErrNoChange {
		retErr = &MigrationError{
			Err:         err,
			Description: "failed to apply migration",
		}
		return retErr
	}

	return retErr
}
