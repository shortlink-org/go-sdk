// Package pgquery classifies statements with PostgreSQL's own parser.
//
// It is a drop-in replacement for the built-in scanner:
//
//	store, err := db.New(ctx, log, tracer, metrics, cfg,
//		postgres.With(postgres.WithClassifier(pgquery.New())),
//	)
//
// # Why this is a separate module
//
// pg_query_go wraps libpg_query, which is PostgreSQL's real parser extracted
// into a C library. That gives a parse tree that matches the server's exactly —
// and it needs cgo, which breaks cross-compilation and CGO_ENABLED=0 builds.
// Forcing that on everyone who imports the db module would be a poor trade for
// a decision the built-in scanner already makes correctly, so this lives in its
// own module: the cost falls only on the people who choose it.
//
// # What it buys, and what it does not
//
// It removes a class of doubt rather than a class of bug. The scanner and the
// parser agree on every case in the scanner's own test table, because those are
// lexical traps — a keyword inside a dollar-quoted body, a nested block
// comment, a data-modifying CTE — and the scanner was written to survive them.
// What the parser gives you is the guarantee that they cannot disagree in a
// case nobody thought to write down.
//
// It does not close the one gap that matters most. Whether
//
//	SELECT audit_and_return(...)
//
// writes is a property of the catalog, not of the grammar, and no parser can
// answer it. Both classifiers fall back to a list of known-volatile functions,
// so both are conservative about the same unknown.
//
// # Cost
//
// Parsing is roughly two orders of magnitude slower than scanning — tens of
// microseconds rather than hundreds of nanoseconds. Wrap it in
// sqlclass.Cached, as New does, and that is paid once per distinct statement
// text rather than once per execution.
package pgquery

import (
	pg "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass"
)

// New returns a classifier backed by PostgreSQL's parser, behind the same
// bounded cache the built-in one uses.
//
//nolint:ireturn // it is a Classifier by construction
func New() sqlclass.Classifier {
	return sqlclass.Cached(sqlclass.ClassifierFunc(Classify))
}

// Classify reports whether a statement may run on a standby.
//
// Anything that fails to parse is Unknown, which routes to the primary. That is
// not a fallback to be embarrassed about: a statement this parser cannot read
// is one the server is unlikely to accept either, and guessing at it would be
// the one mistake worth avoiding.
func Classify(sql string) sqlclass.Class {
	tree, err := pg.Parse(sql)
	if err != nil {
		return sqlclass.Unknown
	}

	if len(tree.GetStmts()) == 0 {
		return sqlclass.Unknown
	}

	// A multi-statement string is answered by its most restrictive member: the
	// whole thing runs on one connection, so one write pins all of it.
	class := sqlclass.Read

	for _, raw := range tree.GetStmts() {
		switch classifyNode(raw.GetStmt()) {
		case sqlclass.Write:
			return sqlclass.Write
		case sqlclass.Unknown:
			class = sqlclass.Unknown
		case sqlclass.Read:
		default:
			return sqlclass.Unknown
		}
	}

	return class
}

// classifyNode answers for one statement.
//
// The allow-list is deliberate. Only the node types that provably cannot write
// are read; everything else — DDL, DML, session state, procedure calls, and any
// node this switch has not heard of — is a write. A parser that grows a node
// type should make this package conservative, not permissive.
func classifyNode(node *pg.Node) sqlclass.Class {
	if node == nil {
		return sqlclass.Unknown
	}

	switch stmt := node.GetNode().(type) {
	case *pg.Node_SelectStmt:
		return classifySelect(stmt.SelectStmt)

	case *pg.Node_ExplainStmt:
		// EXPLAIN ANALYZE executes the plan, and plain EXPLAIN plans against
		// the primary's statistics. Neither is worth routing away.
		return sqlclass.Write

	case *pg.Node_VariableShowStmt:
		return sqlclass.Read

	default:
		return sqlclass.Write
	}
}

// classifySelect walks a SELECT, including the shapes that look like reads and
// are not.
func classifySelect(stmt *pg.SelectStmt) sqlclass.Class {
	if stmt == nil {
		return sqlclass.Unknown
	}

	// SELECT ... INTO creates a table.
	if stmt.GetIntoClause() != nil {
		return sqlclass.Write
	}

	// FOR UPDATE and friends take row locks, which a standby refuses.
	if len(stmt.GetLockingClause()) > 0 {
		return sqlclass.Write
	}

	// A data-modifying CTE: WITH x AS (DELETE ...) SELECT ...
	for _, cte := range stmt.GetWithClause().GetCtes() {
		expr := cte.GetCommonTableExpr()
		if expr == nil {
			return sqlclass.Unknown
		}

		if classifyNode(expr.GetCtequery()) != sqlclass.Read {
			return sqlclass.Write
		}
	}

	// Set operations: both arms have to be reads.
	for _, arm := range []*pg.SelectStmt{stmt.GetLarg(), stmt.GetRarg()} {
		if arm == nil {
			continue
		}

		if classifySelect(arm) != sqlclass.Read {
			return sqlclass.Write
		}
	}

	if callsVolatile(stmt) {
		return sqlclass.Write
	}

	return sqlclass.Read
}

// callsVolatile looks for a call to a function known to write or to read
// session state.
//
// The grammar cannot tell us whether a function writes — that is
// pg_proc.provolatile, a property of the catalog — so this falls back to the
// same list the built-in scanner uses, and is conservative about the same
// unknowns.
func callsVolatile(stmt *pg.SelectStmt) bool {
	found := false

	walk(stmt, func(node *pg.Node) {
		call := node.GetFuncCall()
		if call == nil {
			return
		}

		if sqlclass.IsVolatileCall(names(call)) {
			found = true
		}
	})

	return found
}

// names renders a function call's qualified name, most significant last:
// pg_catalog.nextval yields {"pg_catalog", "nextval"}.
func names(call *pg.FuncCall) []string {
	parts := make([]string, 0, len(call.GetFuncname()))

	for _, node := range call.GetFuncname() {
		if str := node.GetString_(); str != nil {
			parts = append(parts, str.GetSval())
		}
	}

	return parts
}

// The walk helper visits every Node in a protobuf message tree.
//
// The parser ships no walker, and the parse tree is deep and irregular, so this
// reflects over the generated messages rather than enumerating a hundred node
// types by hand — which would go stale the first time PostgreSQL grows a new
// one.
func walk(msg proto.Message, visit func(*pg.Node)) {
	if msg == nil {
		return
	}

	msg.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind {
			return true
		}

		if field.IsList() {
			list := value.List()
			for i := range list.Len() {
				descend(list.Get(i).Message().Interface(), visit)
			}

			return true
		}

		descend(value.Message().Interface(), visit)

		return true
	})
}

func descend(msg proto.Message, visit func(*pg.Node)) {
	if node, ok := msg.(*pg.Node); ok {
		visit(node)
	}

	walk(msg, visit)
}
