//go:build unit

package sqlclass

import "testing"

// The classifier sits in front of every statement, so its cost is added to
// every query in the process. These benchmarks exist to keep it in the noise
// next to a network round trip.

// Statements chosen to cover the real cost range: a short read, a realistic
// joined read, a write, and the two shapes that need a full scan.
var benchStatements = []struct {
	name string
	sql  string
}{
	{name: "select_short", sql: `SELECT 1`},
	{name: "select_join", sql: `SELECT u.id, u.name FROM users u JOIN orders o ON o.user_id = u.id WHERE u.id = $1 ORDER BY o.created_at DESC LIMIT 10`},
	{name: "insert_returning", sql: `INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id`},
	{name: "cte_write", sql: `WITH d AS (DELETE FROM sessions WHERE expires_at < now() RETURNING id) SELECT count(*) FROM d`},
	{name: "select_for_update", sql: `SELECT id FROM jobs WHERE state = 'ready' ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`},
}

// BenchmarkClassifyStatements measures the uncached scanner, i.e. the cost paid
// once per distinct statement text.
func BenchmarkClassifyStatements(b *testing.B) {
	for _, stmt := range benchStatements {
		b.Run(stmt.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				Classify(stmt.sql)
			}
		})
	}
}

// BenchmarkClassifyCached measures what a running service actually pays, since
// statement text is nearly always a package-level constant.
func BenchmarkClassifyCached(b *testing.B) {
	classifier := DefaultClassifier()

	for _, stmt := range benchStatements {
		b.Run(stmt.name, func(b *testing.B) {
			classifier.Classify(stmt.sql)
			b.ReportAllocs()

			for b.Loop() {
				classifier.Classify(stmt.sql)
			}
		})
	}
}
