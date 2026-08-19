package sqlclass

// SQL keywords referenced from more than one place.
const (
	kwUpdate = "UPDATE"
	kwShare  = "SHARE"
	kwBegin  = "BEGIN"
	kwCommit = "COMMIT"
)

// writeKeywords are first tokens that settle the question on their own: the
// statement mutates data, schema, or session state, and no further inspection
// can make it eligible for a standby.
//
// The session-scoped entries deserve their own note. SET, RESET, DISCARD,
// PREPARE, EXECUTE and DEALLOCATE change state that belongs to one connection.
// Running them on a replica configures a session that the following statements
// — which go to the primary — never see, so the statement "succeeds" and the
// caller's intent is quietly lost. That is worse than an error.
var writeKeywords = map[string]bool{
	// data
	"INSERT":   true,
	kwUpdate:   true,
	"DELETE":   true,
	"MERGE":    true,
	"TRUNCATE": true,
	"COPY":     true,

	// schema and maintenance
	"CREATE":   true,
	"ALTER":    true,
	"DROP":     true,
	"COMMENT":  true,
	"RENAME":   true,
	"REINDEX":  true,
	"REFRESH":  true, // REFRESH MATERIALIZED VIEW
	"CLUSTER":  true,
	"VACUUM":   true,
	"ANALYZE":  true,
	"IMPORT":   true,
	"SECURITY": true, // SECURITY LABEL

	// privileges
	"GRANT":  true,
	"REVOKE": true,

	// procedural: a procedure or an anonymous block may do anything at all,
	// including COMMIT.
	"CALL": true,
	"DO":   true,

	// session state
	"SET":        true,
	"RESET":      true,
	"DISCARD":    true,
	"PREPARE":    true,
	"EXECUTE":    true,
	"DEALLOCATE": true,
	"LISTEN":     true,
	"UNLISTEN":   true,
	"NOTIFY":     true,
	"LOCK":       true,

	// transaction control issued as raw SQL. Beyond being a write, it means
	// someone is running a transaction the router cannot follow, so the
	// primary is the only answer that keeps the session coherent.
	kwBegin:      true,
	"START":      true,
	kwCommit:     true,
	"ROLLBACK":   true,
	"END":        true,
	"ABORT":      true,
	"SAVEPOINT":  true,
	"RELEASE":    true,
	"CHECKPOINT": true,
}

// readKeywords are first tokens whose statement may be eligible for a standby,
// subject to the body checks in classifyBody.
var readKeywords = map[string]bool{
	"SELECT": true,
	"VALUES": true, // VALUES (1),(2) is a standalone read
	"TABLE":  true, // TABLE x is SELECT * FROM x
	"SHOW":   true,
}

// Data-modifying keywords, searched for inside a WITH expression.
//
// A CTE that deletes and then selects from the result is idiomatic PostgreSQL,
// and it is exactly what defeats prefix matching.
var dmlKeywords = map[string]bool{
	"INSERT": true,
	kwUpdate: true,
	"DELETE": true,
	"MERGE":  true,
}

// lockModifiers may appear between FOR and the locking keyword, as in
// FOR NO KEY UPDATE.
var lockModifiers = map[string]bool{
	"NO":  true,
	"KEY": true,
}

// volatileFuncs write, or read state that only makes sense on the node that
// produced it. Each is matched as a call — the identifier must be followed by
// an opening parenthesis — so a column named "nextval" is still a read.
var volatileFuncs = map[string]bool{
	// sequences. currval and lastval read, but read *session* state, which is
	// meaningless on a different node.
	"NEXTVAL": true,
	"SETVAL":  true,
	"CURRVAL": true,
	"LASTVAL": true,

	// advisory locks
	"PG_ADVISORY_LOCK":                 true,
	"PG_ADVISORY_LOCK_SHARED":          true,
	"PG_ADVISORY_UNLOCK":               true,
	"PG_ADVISORY_UNLOCK_ALL":           true,
	"PG_ADVISORY_UNLOCK_SHARED":        true,
	"PG_ADVISORY_XACT_LOCK":            true,
	"PG_ADVISORY_XACT_LOCK_SHARED":     true,
	"PG_TRY_ADVISORY_LOCK":             true,
	"PG_TRY_ADVISORY_LOCK_SHARED":      true,
	"PG_TRY_ADVISORY_XACT_LOCK":        true,
	"PG_TRY_ADVISORY_XACT_LOCK_SHARED": true,

	// session configuration
	"SET_CONFIG": true,

	// transaction identity: assigning an xid fails on a standby outright
	"TXID_CURRENT":       true,
	"PG_CURRENT_XACT_ID": true,

	// WAL and replication control
	"PG_CREATE_RESTORE_POINT":             true,
	"PG_SWITCH_WAL":                       true,
	"PG_LOGICAL_EMIT_MESSAGE":             true,
	"PG_REPLICATION_ORIGIN_ADVANCE":       true,
	"PG_REPLICATION_ORIGIN_SESSION_SETUP": true,

	// side effects reaching outside the current query
	"PG_NOTIFY":   true,
	"DBLINK_EXEC": true,
}

// stringPrefixes are identifier-looking prefixes that introduce a string
// literal. Only E enables backslash escapes; with standard_conforming_strings
// on — the default since 9.1 — a backslash in a plain literal is just a
// backslash, and treating it as an escape would run the scanner past the
// closing quote.
var stringPrefixes = map[string]bool{
	"E": true,
	"B": true,
	"X": true,
}
