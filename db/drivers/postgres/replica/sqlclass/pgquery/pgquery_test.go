//go:build unit

package pgquery_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass/pgquery"
)

// The traps the built-in scanner was written to survive. Running the parser
// over the same table is the point of this package: if the two ever disagree
// here, one of them is wrong.
//
//nolint:funlen // one table, one trap per case
func TestClassifyMatchesTheScanner(t *testing.T) {
	t.Parallel()

	cases := []string{
		// plain reads
		`SELECT 1`,
		`SELECT u.id FROM users u JOIN orders o ON o.user_id = u.id WHERE u.id = $1`,
		`VALUES (1),(2)`,
		`TABLE users`,
		`SHOW statement_timeout`,
		`SELECT 1;`,

		// comments
		"-- hi\nSELECT 1",
		`/* hi */ SELECT 1`,
		`/* a /* b */ c */ SELECT 1`,

		// literals must not leak keywords
		`SELECT 'INSERT INTO x'`,
		`SELECT 'it''s INSERT'`,
		`SELECT $$INSERT INTO x$$`,
		`SELECT $tag$UPDATE y$tag$`,
		`SELECT "update" FROM t`,
		`SELECT * FROM t WHERE id = $1 AND name = $2`,

		// writes
		`INSERT INTO t VALUES (1)`,
		`INSERT INTO t VALUES (1) RETURNING id`,
		`UPDATE t SET a = 1`,
		`DELETE FROM t`,
		`TRUNCATE t`,
		`CREATE TABLE t (id int)`,
		`ALTER TABLE t ADD COLUMN a int`,
		`DROP TABLE t`,
		`REFRESH MATERIALIZED VIEW CONCURRENTLY mv`,
		`CALL do_thing()`,
		`DO $$ BEGIN PERFORM 1; END $$`,
		`GRANT SELECT ON t TO app`,

		// session state
		`SET application_name = 'x'`,
		`SET LOCAL statement_timeout = '1s'`,
		`RESET ALL`,
		`DISCARD ALL`,
		`PREPARE p AS SELECT 1`,
		`EXECUTE p`,
		`DEALLOCATE p`,
		`LISTEN chan`,
		`LOCK TABLE t`,

		// transaction control
		`BEGIN`,
		`COMMIT`,
		`ROLLBACK TO SAVEPOINT s`,

		// data-modifying CTEs
		`WITH d AS (DELETE FROM t RETURNING *) SELECT * FROM d`,
		`WITH i AS (INSERT INTO t VALUES (1) RETURNING id) SELECT * FROM i`,
		`WITH u AS (UPDATE t SET a = 1 RETURNING *) SELECT * FROM u`,
		`WITH s AS (SELECT 1) INSERT INTO t SELECT * FROM s`,
		`WITH s AS (SELECT 1) SELECT * FROM s`,
		`WITH RECURSIVE r AS (SELECT 1) SELECT * FROM r`,
		`WITH s AS (SELECT 'DELETE') SELECT * FROM s`,

		// locking clauses
		`SELECT * FROM t FOR UPDATE`,
		`SELECT * FROM t FOR NO KEY UPDATE`,
		`SELECT * FROM t FOR SHARE`,
		`SELECT * FROM t FOR KEY SHARE`,
		`SELECT * FROM t FOR UPDATE OF t SKIP LOCKED`,

		// SELECT INTO creates a table
		`SELECT * INTO new FROM t`,

		// volatile functions
		`SELECT nextval('s')`,
		`SELECT pg_catalog.nextval('s')`,
		`SELECT nextval FROM t`,
		`SELECT t.nextval FROM t`,
		`SELECT setval('s', 1)`,
		`SELECT currval('s')`,
		`SELECT pg_advisory_xact_lock(1)`,
		`SELECT set_config('a', 'b', true)`,
		`SELECT txid_current()`,
		`SELECT pg_notify('c', 'p')`,
		`SELECT 'nextval(' FROM t`,

		// EXPLAIN goes to the primary either way
		`EXPLAIN SELECT 1`,
		`EXPLAIN ANALYZE INSERT INTO t VALUES (1)`,

		// unanalysable
		``,
		`   `,
		`FROBNICATE t`,
		`¯\_(ツ)_/¯`,
	}

	for _, sql := range cases {
		t.Run(sql, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, sqlclass.Classify(sql), pgquery.Classify(sql),
				"scanner and parser disagree on: %s", sql)
		})
	}
}

// Where the two legitimately differ, and why. Both directions are safe — a
// disagreement that resolved toward Read would not be.
func TestClassifyDivergesOnlyTowardCaution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sql     string
		scanner sqlclass.Class
		parser  sqlclass.Class
		why     string
	}{
		{
			name:    "multi statement",
			sql:     `SELECT 1; SELECT 2`,
			scanner: sqlclass.Unknown,
			parser:  sqlclass.Read,
			why:     "the parser reads both statements; the scanner stops at the semicolon and declines to guess",
		},
		{
			name:    "routing hint",
			sql:     `/* route:read */ SELECT * FROM t FOR UPDATE`,
			scanner: sqlclass.Read,
			parser:  sqlclass.Write,
			why:     "hints are a comment convention, and a parser discards comments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.scanner, sqlclass.Classify(tt.sql), tt.why)
			assert.Equal(t, tt.parser, pgquery.Classify(tt.sql), tt.why)
		})
	}
}

func TestClassifyNeverPanics(t *testing.T) {
	t.Parallel()

	for _, sql := range []string{`SELECT '`, `SELECT $tag$ oops`, `/*`, `--`, "SELECT \x00 FROM t"} {
		assert.NotPanics(t, func() { pgquery.Classify(sql) }, "sql: %q", sql)
	}
}

func BenchmarkClassify(b *testing.B) {
	const sql = `SELECT u.id, u.name FROM users u JOIN orders o ON o.user_id = u.id WHERE u.id = $1 ORDER BY o.created_at DESC LIMIT 10`

	b.Run("parser", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			pgquery.Classify(sql)
		}
	})

	b.Run("scanner", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			sqlclass.Classify(sql)
		}
	})

	b.Run("parser_cached", func(b *testing.B) {
		classifier := pgquery.New()
		classifier.Classify(sql)

		b.ReportAllocs()

		for b.Loop() {
			classifier.Classify(sql)
		}
	})
}
