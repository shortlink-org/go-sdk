# pgquery

Classifies statements with PostgreSQL's own parser instead of the built-in
scanner.

```go
import "github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass/pgquery"

store, err := db.New(ctx, log, tracer, metrics, cfg,
	postgres.With(postgres.WithClassifier(pgquery.New())),
)
```

## Why a separate module

`pg_query_go` wraps libpg_query — PostgreSQL's real parser extracted into a C
library — so the parse tree matches the server's exactly. It also needs **cgo**,
which breaks cross-compilation and `CGO_ENABLED=0` builds.

Putting that in the `db` module would impose it on everyone. A separate module
means the cost lands only on the people who choose it: its own `go.mod`, and
nothing in `db`'s dependency graph.

It is deliberately **not** in the repository's `go.work`. Adding it there would
pull cgo and libpg_query into the shared workspace and break the root vendor
directory for every other module — which is exactly the imposition the split
exists to avoid. Build and test it from its own directory:

```bash
cd db/drivers/postgres/replica/sqlclass/pgquery
GOWORK=off go test -tags unit ./...
```

## What it buys

It removes a class of doubt, not a class of bug. The conformance test runs both
classifiers over the same seventy-odd cases — dollar-quoted keywords, nested
block comments, data-modifying CTEs, locking clauses, volatile function calls —
and **they agree on every one**. The scanner was written for exactly those
traps.

What the parser adds is the guarantee that the two cannot diverge on a case
nobody thought to write down.

Where they differ, and why:

| statement | scanner | parser |
|---|---|---|
| `SELECT 1; SELECT 2` | unknown | read — the parser reads both; the scanner stops at the semicolon and declines to guess |
| `/* route:read */ SELECT … FOR UPDATE` | read | write — hints are a comment convention, and a parser discards comments |

Both directions are safe. A divergence that resolved *toward* read would not be.

## What it does not buy

Whether

```sql
SELECT audit_and_return(...)
```

writes is `pg_proc.provolatile` — a property of the catalog, not of the grammar.
No parser can answer it from the statement alone. Both classifiers fall back to
the same list of known-volatile functions, so both are conservative about the
same unknown. Closing that gap means reading the catalog at startup, not
swapping the classifier.

## Cost

| | ns/op | allocs |
|---|---|---|
| scanner | 481 | 8 |
| parser | 28 947 | 204 |
| parser, cached | 10.6 | 0 |

Sixty times slower uncached, and irrelevant once cached — which `New` does for
you. Statement text is nearly always a package-level constant, so the parse is
paid once per distinct statement rather than once per execution.

## Alternatives

`pgplex/pgparser` is a pure-Go port of PostgreSQL's own `gram.y`, reporting
99.6% against the server's regression suite. It would make this adapter
cgo-free. At 32 stars and 36 commits it is not yet something to hand every
query in production to — but the adapter is a hundred lines, so a second one is
cheap when it matures.

See [ADR 0001 — Read-replica routing](../../../ADR/0001-read-replica-routing.md).
