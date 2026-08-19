package replica

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Query, QueryRow and SendBatch have nowhere to put a routing error: the first
// must return a pgx.Rows, the second returns only a pgx.Row, and the third
// returns only pgx.BatchResults. The pgxpool package has the same problem and
// solves it with error-carrying implementations — but they are unexported, so
// the router carries its own.
//
// Returning a nil pgx.Rows alongside an error is not an option:
//
//	rows, err := pool.Query(ctx, sql)
//	defer rows.Close()   // written before the error check more often than not
//
// A nil there is a panic in a deferred call, which is about the least helpful
// place a panic can happen.

// errRows is a pgx.Rows that yields nothing and reports err.
type errRows struct{ err error }

var _ pgx.Rows = errRows{}

func (r errRows) Close()                                       {}
func (r errRows) Err() error                                   { return r.err }
func (r errRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r errRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r errRows) Next() bool                                   { return false }
func (r errRows) Scan(...any) error                            { return r.err }
func (r errRows) Values() ([]any, error)                       { return nil, r.err }
func (r errRows) RawValues() [][]byte                          { return nil }
func (r errRows) Conn() *pgx.Conn                              { return nil }

// errRow is a pgx.Row that reports err from Scan, which is the only place the
// caller can see it.
type errRow struct{ err error }

var _ pgx.Row = errRow{}

func (r errRow) Scan(...any) error { return r.err }

// errBatchResults is a pgx.BatchResults that reports err from every call.
type errBatchResults struct{ err error }

var _ pgx.BatchResults = errBatchResults{}

func (b errBatchResults) Exec() (pgconn.CommandTag, error) { return pgconn.CommandTag{}, b.err }

//nolint:ireturn // implements pgx.BatchResults
func (b errBatchResults) Query() (pgx.Rows, error) { return errRows(b), b.err }

//nolint:ireturn // implements pgx.BatchResults
func (b errBatchResults) QueryRow() pgx.Row { return errRow(b) }
func (b errBatchResults) Close() error      { return b.err }
