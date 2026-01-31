//go:build integration

package migrate_test

import (
	"context"
	"embed"
	"io/fs"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/migrate"
)

//go:embed fixtures/migrations/*.sql
var fixturesFS embed.FS

// testMigrations returns an FS rooted at fixtures/ so that "migrations" path works
func testMigrations() fs.FS {
	sub, _ := fs.Sub(fixturesFS, "fixtures")
	return sub
}

// testDB implements db.DB interface for testing
type testDB struct {
	pool *pgxpool.Pool
}

func (t *testDB) Init(_ context.Context) error { return nil }
func (t *testDB) GetConn() any                 { return t.pool }

func setupPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()
		container.Terminate(context.Background())
	})

	return pool
}

func TestMigration_AppliesAndCloses(t *testing.T) {
	pool := setupPostgres(t)
	ctx := context.Background()

	db := &testDB{pool: pool}

	err := migrate.Migration(ctx, db, testMigrations(), "test_table")
	require.NoError(t, err)

	// Verify migration was applied
	var exists bool
	err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'test_users')").Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists, "table test_users should exist after migration")

	// Verify pool can be closed without hanging
	done := make(chan struct{})
	go func() {
		pool.Close()
		close(done)
	}()

	select {
	case <-done:
		// OK - closed successfully
	case <-time.After(5 * time.Second):
		t.Fatal("pool.Close() hung - migration did not properly release connections")
	}
}

func TestMigration_NoChange(t *testing.T) {
	pool := setupPostgres(t)
	ctx := context.Background()

	db := &testDB{pool: pool}

	// Apply migration twice - second should be no-op
	err := migrate.Migration(ctx, db, testMigrations(), "test_table")
	require.NoError(t, err)

	err = migrate.Migration(ctx, db, testMigrations(), "test_table")
	require.NoError(t, err)
}
