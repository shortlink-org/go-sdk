//go:build unit

package sqlclass

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Statements reused across the table, named so that the linter's literal count
// stays bounded and a reader can see at a glance which case is which.
const (
	sqlSelectOne = `SELECT 1`
	sqlUpdate    = `UPDATE t SET a = 1`
)

// caseEmpty names the "no input at all" case, which several tables share.
const caseEmpty = "empty"

//nolint:funlen,maintidx // one table, one case per rule the classifier implements
func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sql  string
		want Class
	}{
		// --- plain reads ---
		{name: "select", sql: sqlSelectOne, want: Read},
		{name: "select lowercase", sql: `select 1`, want: Read},
		{name: "select mixed case", sql: `SeLeCt 1`, want: Read},
		{name: "leading whitespace", sql: "\n\t  SELECT 1", want: Read},
		{name: "values", sql: `VALUES (1),(2)`, want: Read},
		{name: "table", sql: `TABLE users`, want: Read},
		{name: "show", sql: `SHOW statement_timeout`, want: Read},
		{name: "join with placeholders", sql: `SELECT u.id FROM users u JOIN orders o ON o.user_id = u.id WHERE u.id = $1`, want: Read},
		{name: "trailing semicolon", sql: `SELECT 1;`, want: Read},
		{name: "trailing semicolon and comment", sql: "SELECT 1; -- done", want: Read},

		// --- comments ---
		{name: "leading line comment", sql: "-- hi\nSELECT 1", want: Read},
		{name: "leading block comment", sql: `/* hi */ SELECT 1`, want: Read},
		{name: "nested block comment", sql: `/* a /* b */ c */ SELECT 1`, want: Read},
		{name: "unterminated block comment", sql: `/* SELECT 1`, want: Unknown},

		// --- routing hints ---
		{name: "hint read overrides lock", sql: `/* route:read */ SELECT * FROM t FOR UPDATE`, want: Read},
		{name: "hint write overrides select", sql: `/* route:write */ SELECT 1`, want: Write},
		{name: "hint in line comment", sql: "-- route:write\nSELECT 1", want: Write},
		{name: "hint case insensitive", sql: `/* ROUTE:WRITE */ SELECT 1`, want: Write},
		{name: "first hint wins", sql: `/* route:read */ /* route:write */ SELECT 1 FOR UPDATE`, want: Read},

		// --- literals must not leak keywords ---
		{name: "insert inside string", sql: `SELECT 'INSERT INTO x'`, want: Read},
		{name: "insert inside escaped string", sql: `SELECT E'INSERT \' INTO x'`, want: Read},
		{name: "doubled quote in string", sql: `SELECT 'it''s INSERT'`, want: Read},
		{name: "backslash in standard string", sql: `SELECT 'c:\', id FROM t`, want: Read},
		{name: "insert in dollar quote", sql: `SELECT $$INSERT INTO x$$`, want: Read},
		{name: "update in tagged dollar quote", sql: `SELECT $tag$UPDATE y$tag$`, want: Read},
		{name: "keyword as quoted identifier", sql: `SELECT "update" FROM t`, want: Read},
		{name: "placeholder is not a dollar quote", sql: `SELECT * FROM t WHERE id = $1 AND name = $2`, want: Read},

		// --- writes by first keyword ---
		{name: "insert", sql: `INSERT INTO t VALUES (1)`, want: Write},
		{name: "insert returning", sql: `INSERT INTO t VALUES (1) RETURNING id`, want: Write},
		{name: "update", sql: sqlUpdate, want: Write},
		{name: "delete", sql: `DELETE FROM t`, want: Write},
		{name: "merge", sql: `MERGE INTO t USING s ON t.id = s.id`, want: Write},
		{name: "truncate", sql: `TRUNCATE t`, want: Write},
		{name: "copy", sql: `COPY t FROM STDIN`, want: Write},
		{name: "create", sql: `CREATE TABLE t (id int)`, want: Write},
		{name: "alter", sql: `ALTER TABLE t ADD COLUMN a int`, want: Write},
		{name: "drop", sql: `DROP TABLE t`, want: Write},
		{name: "refresh matview", sql: `REFRESH MATERIALIZED VIEW CONCURRENTLY mv`, want: Write},
		{name: "call", sql: `CALL do_thing()`, want: Write},
		{name: "do block", sql: `DO $$ BEGIN PERFORM 1; END $$`, want: Write},
		{name: "grant", sql: `GRANT SELECT ON t TO app`, want: Write},
		{name: "vacuum", sql: `VACUUM ANALYZE t`, want: Write},

		// --- session state is a write, even when it reads like one isn't ---
		{name: "set", sql: `SET application_name = 'x'`, want: Write},
		{name: "set local", sql: `SET LOCAL statement_timeout = '1s'`, want: Write},
		{name: "reset", sql: `RESET ALL`, want: Write},
		{name: "discard", sql: `DISCARD ALL`, want: Write},
		{name: "prepare", sql: `PREPARE p AS SELECT 1`, want: Write},
		{name: "execute", sql: `EXECUTE p`, want: Write},
		{name: "deallocate", sql: `DEALLOCATE p`, want: Write},
		{name: "listen", sql: `LISTEN chan`, want: Write},
		{name: "lock", sql: `LOCK TABLE t`, want: Write},

		// --- transaction control as raw SQL ---
		{name: "begin", sql: `BEGIN`, want: Write},
		{name: "begin read only", sql: `BEGIN TRANSACTION READ ONLY`, want: Write},
		{name: "commit", sql: `COMMIT`, want: Write},
		{name: "rollback to savepoint", sql: `ROLLBACK TO SAVEPOINT s`, want: Write},

		// --- data-modifying CTEs ---
		{name: "cte delete", sql: `WITH d AS (DELETE FROM t RETURNING *) SELECT * FROM d`, want: Write},
		{name: "cte insert", sql: `WITH i AS (INSERT INTO t VALUES (1) RETURNING id) SELECT * FROM i`, want: Write},
		{name: "cte update", sql: `WITH u AS (UPDATE t SET a = 1 RETURNING *) SELECT * FROM u`, want: Write},
		{name: "cte then insert", sql: `WITH s AS (SELECT 1) INSERT INTO t SELECT * FROM s`, want: Write},
		{name: "cte read only", sql: `WITH s AS (SELECT 1) SELECT * FROM s`, want: Read},
		{name: "cte recursive read", sql: `WITH RECURSIVE r AS (SELECT 1) SELECT * FROM r`, want: Read},
		{name: "cte dml keyword inside string", sql: `WITH s AS (SELECT 'DELETE') SELECT * FROM s`, want: Read},

		// --- locking clauses ---
		{name: "for update", sql: `SELECT * FROM t FOR UPDATE`, want: Write},
		{name: "for no key update", sql: `SELECT * FROM t FOR NO KEY UPDATE`, want: Write},
		{name: "for share", sql: `SELECT * FROM t FOR SHARE`, want: Write},
		{name: "for key share", sql: `SELECT * FROM t FOR KEY SHARE`, want: Write},
		{name: "for update of skip locked", sql: `SELECT * FROM t FOR UPDATE OF t SKIP LOCKED`, want: Write},

		// --- SELECT INTO creates a table ---
		{name: "select into", sql: `SELECT * INTO new FROM t`, want: Write},

		// --- volatile functions ---
		{name: "nextval", sql: `SELECT nextval('s')`, want: Write},
		{name: "nextval with space", sql: `SELECT nextval ('s')`, want: Write},
		{name: "nextval qualified by pg_catalog", sql: `SELECT pg_catalog.nextval('s')`, want: Write},
		{name: "column named nextval", sql: `SELECT nextval FROM t`, want: Read},
		{name: "column nextval qualified by table", sql: `SELECT t.nextval FROM t`, want: Read},
		{name: "setval", sql: `SELECT setval('s', 1)`, want: Write},
		{name: "currval reads session state", sql: `SELECT currval('s')`, want: Write},
		{name: "advisory xact lock", sql: `SELECT pg_advisory_xact_lock(1)`, want: Write},
		{name: "try advisory lock", sql: `SELECT pg_try_advisory_lock(1)`, want: Write},
		{name: "set_config", sql: `SELECT set_config('a', 'b', true)`, want: Write},
		{name: "txid_current", sql: `SELECT txid_current()`, want: Write},
		{name: "pg_notify", sql: `SELECT pg_notify('c', 'p')`, want: Write},
		{name: "volatile name inside string", sql: `SELECT 'nextval(' FROM t`, want: Read},

		// --- EXPLAIN goes to the primary wholesale ---
		{name: "explain", sql: `EXPLAIN SELECT 1`, want: Write},
		{name: "explain analyze insert", sql: `EXPLAIN ANALYZE INSERT INTO t VALUES (1)`, want: Write},
		{name: "explain with options", sql: `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT 1`, want: Write},

		// --- unanalysable ---
		{name: "multi statement", sql: `SELECT 1; SELECT 2`, want: Unknown},
		{name: "multi statement with write", sql: `SELECT 1; DELETE FROM t`, want: Unknown},
		{name: caseEmpty, sql: ``, want: Unknown},
		{name: "whitespace only", sql: "  \n\t ", want: Unknown},
		{name: "comment only", sql: `/* nothing here */`, want: Unknown},
		{name: "starts with punctuation", sql: `(SELECT 1)`, want: Unknown},
		{name: "starts with a number", sql: `1`, want: Unknown},
		{name: "unknown keyword", sql: `FROBNICATE t`, want: Unknown},
		{name: "not sql at all", sql: `¯\_(ツ)_/¯`, want: Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Classify(tt.sql), "sql: %s", tt.sql)
		})
	}
}

// TestClassifyNeverPanics feeds the scanner deliberately broken input. A
// classifier that panics takes the process down on a statement the caller only
// wanted an error for.
func TestClassifyNeverPanics(t *testing.T) {
	t.Parallel()

	inputs := []string{
		`SELECT '`,           // unterminated string
		`SELECT "`,           // unterminated identifier
		`SELECT $tag$ oops`,  // unterminated dollar quote
		`SELECT $`,           // bare dollar at end
		`SELECT E'\`,         // escape at end of input
		`/*`,                 // unterminated comment
		`/* /* */`,           // unbalanced nesting
		`--`,                 // bare line comment
		"SELECT \x00 FROM t", // NUL byte
		strings.Repeat("(", 10000),
		strings.Repeat("SELECT 1; ", 1000),
	}

	for _, sql := range inputs {
		assert.NotPanics(t, func() { Classify(sql) }, "sql: %q", sql)
	}
}

func TestCachingClassifier(t *testing.T) {
	t.Parallel()

	calls := 0
	inner := ClassifierFunc(func(string) Class {
		calls++

		return Read
	})

	classifier := newCachingClassifier(inner, 2)

	assert.Equal(t, Read, classifier.Classify(sqlSelectOne))
	assert.Equal(t, Read, classifier.Classify(sqlSelectOne))
	assert.Equal(t, 1, calls, "second lookup should be served from the cache")

	// Exceeding the cap drops the cache wholesale rather than growing it.
	classifier.Classify(`SELECT 2`)
	classifier.Classify(`SELECT 3`)
	assert.LessOrEqual(t, len(classifier.cache), 2)
}

func TestClassString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "read", Read.String())
	assert.Equal(t, "write", Write.String())
	assert.Equal(t, "unknown", Unknown.String())
}

func BenchmarkClassify(b *testing.B) {
	const sql = `SELECT u.id, u.name FROM users u JOIN orders o ON o.user_id = u.id WHERE u.id = $1 ORDER BY o.created_at DESC LIMIT 10`

	b.ReportAllocs()

	for b.Loop() {
		Classify(sql)
	}
}
