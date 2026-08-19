// Package wal models PostgreSQL Write-Ahead Log positions.
//
// It depends on nothing beyond the standard library. A WAL position is a
// number and a token is a string; everything that routes on them lives
// elsewhere.
package wal

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// LSN is a PostgreSQL Write-Ahead Log position: the byte offset of a record in
// the WAL stream, which the server renders as two hex halves separated by a
// slash ("16/B374D848").
//
// The zero value means "no position known". Ordering is plain integer
// comparison, and that is what makes read-after-write cheap: a replica has
// replayed a write once its replay position reaches the position captured
// after that write.
type LSN uint64

// Unknown marks a write whose WAL position could not be determined. It
// compares greater than any position a server can report, so a read gated on
// it never sees a replica as caught up: the routing degrades to the primary
// instead of silently serving stale data.
const Unknown LSN = math.MaxUint64

// tokenVersion prefixes every encoded Token. It exists so that a later change
// to the wire shape can be recognized and discarded rather than misparsed —
// tokens travel through clients we do not control.
const tokenVersion = "v1"

// maxTokenLen bounds what ParseToken will look at. A token is at most ~70
// bytes; anything longer arrived from a client that is not playing along.
const maxTokenLen = 128

// A WAL position is two 32-bit halves rendered in hex, and a token has five
// colon-separated fields.
const (
	hexBase      = 16
	decimalBase  = 10
	halfBits     = 32
	systemIDBits = 64
	timelineBits = 32
	millisBits   = 64
	tokenFields  = 5
)

var (
	// ErrInvalidLSN indicates a string that is not a PostgreSQL WAL position.
	ErrInvalidLSN = errors.New("invalid PostgreSQL LSN")
	// ErrInvalidToken indicates a watermark token that could not be decoded.
	ErrInvalidToken = errors.New("invalid watermark token")
)

// ParseLSN parses the "XXXXXXXX/XXXXXXXX" text form the server produces.
//
// The pgx driver has no codec for pg_lsn, so every query that reads a WAL
// position casts it to text and lands here.
func ParseLSN(text string) (LSN, error) {
	slash := strings.IndexByte(text, '/')
	if slash < 1 || slash == len(text)-1 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidLSN, text)
	}

	high, err := strconv.ParseUint(text[:slash], hexBase, halfBits)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidLSN, text)
	}

	low, err := strconv.ParseUint(text[slash+1:], hexBase, halfBits)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidLSN, text)
	}

	return LSN(high<<halfBits | low), nil
}

// String renders the canonical text form, matching what the server prints.
func (l LSN) String() string {
	return fmt.Sprintf("%X/%X", uint64(l)>>halfBits, uint64(l)&math.MaxUint32)
}

// Token is a watermark that survives a process boundary.
//
// The system identifier and timeline are not decoration. After a failover an
// LSN from the previous timeline names a WAL position that either never
// existed on the new primary or now holds unrelated records, so comparing it
// against the new replay position is meaningless. A token from a different
// lineage must be discarded, not trusted, and that is only possible if the
// lineage travels with the position.
type Token struct {
	SystemID uint64
	Timeline uint32
	LSN      LSN
	IssuedAt time.Time
}

// String renders "v1:<system-id>:<timeline>:<lsn>:<unix-millis>". The form is
// deliberately opaque-looking and free of separators that would need escaping
// in an HTTP header or a message metadata value.
func (t Token) String() string {
	return fmt.Sprintf("%s:%d:%d:%s:%d",
		tokenVersion, t.SystemID, t.Timeline, t.LSN, t.IssuedAt.UnixMilli())
}

// ParseToken decodes the text form. It is total: every malformed input yields
// ErrInvalidToken rather than a partially filled Token, because callers feed
// it values taken straight from a request header or a cookie.
func ParseToken(text string) (Token, error) {
	if len(text) > maxTokenLen {
		return Token{}, fmt.Errorf("%w: too long (%d bytes)", ErrInvalidToken, len(text))
	}

	parts := strings.Split(text, ":")
	if len(parts) != tokenFields || parts[0] != tokenVersion {
		return Token{}, fmt.Errorf("%w: %q", ErrInvalidToken, text)
	}

	systemID, err := strconv.ParseUint(parts[1], decimalBase, systemIDBits)
	if err != nil {
		return Token{}, fmt.Errorf("%w: system id: %q", ErrInvalidToken, parts[1])
	}

	timeline, err := strconv.ParseUint(parts[2], decimalBase, timelineBits)
	if err != nil {
		return Token{}, fmt.Errorf("%w: timeline: %q", ErrInvalidToken, parts[2])
	}

	lsn, err := ParseLSN(parts[3])
	if err != nil {
		return Token{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	millis, err := strconv.ParseInt(parts[4], decimalBase, millisBits)
	if err != nil {
		return Token{}, fmt.Errorf("%w: issued at: %q", ErrInvalidToken, parts[4])
	}

	return Token{
		SystemID: systemID,
		Timeline: uint32(timeline), //nolint:gosec // ParseUint was given a 32-bit size
		LSN:      lsn,
		IssuedAt: time.UnixMilli(millis),
	}, nil
}

// IsZero reports whether the token carries no position.
func (t Token) IsZero() bool {
	return t.SystemID == 0 && t.Timeline == 0 && t.LSN == 0
}

// SameLineage reports whether two tokens describe the same cluster on the same
// timeline, i.e. whether their LSNs are comparable at all.
func (t Token) SameLineage(other Token) bool {
	return t.SystemID == other.SystemID && t.Timeline == other.Timeline
}
