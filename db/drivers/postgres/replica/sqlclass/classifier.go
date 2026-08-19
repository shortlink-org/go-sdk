// Package sqlclass decides whether a SQL statement may run on a read replica.
//
// It answers a boolean with an asymmetric cost, not "what does this statement
// mean": being wrong about a read is silent, being wrong about a write is
// loud, so anything it does not recognize is reported as unknown and the
// caller sends it to the primary.
//
// A single-pass byte scanner rather than a parser, because the traps are
// lexical rather than grammatical — a keyword inside a dollar-quoted body, a
// nested block comment, a locking clause at the end of a SELECT — and because
// a full grammar would still not answer the one question that matters and
// cannot be answered syntactically: whether a user-defined function writes.
package sqlclass

import (
	"strings"
	"sync"
)

// Class is what the router learns about a statement before it runs.
type Class uint8

const (
	// Unknown means the statement could not be understood. It is treated
	// as a write, because every way of being wrong about a read is silent and
	// every way of being wrong about a write is loud.
	Unknown Class = iota
	// Read means the statement may run on a standby.
	Read
	// Write means the statement must run on the primary.
	Write
)

// String implements fmt.Stringer. The values are used as metric attributes, so
// they are lowercase and stable.
func (c Class) String() string {
	switch c {
	case Read:
		return readName
	case Write:
		return writeName
	case Unknown:
		return unknownName
	default:
		return unknownName
	}
}

// Classifier decides whether a statement may run on a replica.
type Classifier interface {
	Classify(sql string) Class
}

// ClassifierFunc adapts a function to Classifier.
type ClassifierFunc func(sql string) Class

// Classify implements Classifier.
func (f ClassifierFunc) Classify(sql string) Class { return f(sql) }

// Hint comments let a caller override the classification for one statement.
// They travel with the SQL rather than with the context, which is what you
// want when the statement is a package-level constant, and they show up in
// pg_stat_statements, which is what you want when you are trying to work out
// why a query went where it did.
//
//	/* route:read */  SELECT ... FOR UPDATE   -- I know; it is a lock-free view
//	/* route:write */ SELECT refresh_cache()  -- volatile function we don't list
const (
	hintRead  = "route:read"
	hintWrite = "route:write"
)

// maxCachedStatements bounds the classification cache. The cap is not
// optional: a service that builds SQL by concatenating values produces an
// unbounded set of distinct strings, and an uncapped cache turns that into a
// memory leak with the classifier's name on it.
const maxCachedStatements = 4096

// asciiMax is the first byte value outside ASCII. PostgreSQL allows non-ASCII
// identifiers, and treating those bytes as identifier characters keeps a
// UTF-8 name from being chopped into punctuation.
const asciiMax = 0x80

// Names of the classes, as they appear in metric attributes.
const (
	readName    = "read"
	writeName   = "write"
	unknownName = "unknown"
)

// Classify reports whether a statement may run on a standby. It is
// deliberately pessimistic: anything it does not recognize is Unknown,
// which routes to the primary.
//
// One pass over the bytes, around half a microsecond for a typical query. That
// is per distinct statement, not per execution — DefaultClassifier caches, and
// statement text is nearly always a package-level constant.
func Classify(sql string) Class {
	scan := &scanner{src: sql}

	scan.skipTrivia()

	if scan.hintSet {
		return scan.hint
	}

	text, kind := scan.next()
	if kind != tokWord {
		return Unknown
	}

	switch {
	case writeKeywords[text]:
		return Write

	case text == "EXPLAIN":
		// EXPLAIN ANALYZE executes the plan, and the option-list grammar
		// (EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) ...) is the fiddliest part
		// of the whole classifier. EXPLAIN never runs in a hot path, so the
		// throughput it would win does not pay for the risk of getting the
		// grammar wrong.
		return Write

	case text == "WITH":
		// A CTE may modify data. Scan the whole statement for DML before
		// applying the ordinary read-body rules.
		return scan.classifyBody(true)

	case readKeywords[text]:
		return scan.classifyBody(false)

	default:
		return Unknown
	}
}

// DefaultClassifier returns Classify behind a bounded cache. Statement text is
// almost always a package-level constant, so the cache hits nearly always.
//
//nolint:ireturn,iface // the point is that callers can swap in their own
func DefaultClassifier() Classifier {
	return Cached(ClassifierFunc(Classify))
}

// Cached wraps any classifier in the same bounded cache.
//
// It is exported for the alternative implementations: a parser-backed
// classifier costs tens of microseconds where this one costs hundreds of
// nanoseconds, and the cache is what makes that difference irrelevant — the
// price is paid once per distinct statement text, not once per execution.
//
//nolint:ireturn,iface // it decorates a Classifier, so it must be one
func Cached(inner Classifier) Classifier {
	return newCachingClassifier(inner, maxCachedStatements)
}

// IsVolatileCall reports whether a qualified function name is one that writes,
// or reads state that only means something on the node that produced it.
//
// The name arrives most significant last, as the parser yields it:
// {"pg_catalog", "nextval"}. A qualifier other than pg_catalog means a
// user-defined function that happens to share the name, which is not the one
// we are guarding against.
//
// Whether an arbitrary function writes is pg_proc.provolatile, a property of
// the catalog rather than of the syntax, so no classifier — scanner or parser —
// can answer it from the statement alone. This list is where both stop.
func IsVolatileCall(parts []string) bool {
	if len(parts) == 0 {
		return false
	}

	if len(parts) > 1 && !strings.EqualFold(parts[len(parts)-2], "pg_catalog") {
		return false
	}

	return volatileFuncs[asciiUpper(parts[len(parts)-1])]
}

type cachingClassifier struct {
	inner    Classifier
	capacity int

	mu    sync.RWMutex
	cache map[string]Class
}

func newCachingClassifier(inner Classifier, capacity int) *cachingClassifier {
	return &cachingClassifier{
		inner:    inner,
		capacity: capacity,
		cache:    make(map[string]Class),
	}
}

// Classify implements Classifier.
func (c *cachingClassifier) Classify(sql string) Class {
	c.mu.RLock()
	class, ok := c.cache[sql]
	c.mu.RUnlock()

	if ok {
		return class
	}

	class = c.inner.Classify(sql)

	c.mu.Lock()
	// Drop everything rather than evict one entry. There is no per-lookup
	// bookkeeping to pay for, and the working set — a fixed collection of
	// constant strings — refills within a handful of requests.
	if len(c.cache) >= c.capacity {
		c.cache = make(map[string]Class, c.capacity)
	}

	c.cache[sql] = class
	c.mu.Unlock()

	return class
}

type tokenKind uint8

const (
	tokEOF tokenKind = iota
	tokWord
	tokPunct
	tokString
	tokQuotedIdent
	tokNumber
)

// A scanner walks SQL one byte at a time, skipping strings, comments and
// dollar-quoted bodies as single units.
//
// A tokenizer rather than a regex because PostgreSQL's lexical rules defeat
// regexes outright: block comments nest (/* a /* b */ c */ is one comment) and
// dollar-quoted bodies ($tag$ ... $tag$) may contain anything at all, keywords
// included.
type scanner struct {
	src string
	pos int

	hint    Class
	hintSet bool
}

// The classifyBody method walks the rest of the statement, demoting an apparent
// read to a write on anything that mutates, and with checkDML set it also looks
// for a data-modifying CTE.
func (sc *scanner) classifyBody(checkDML bool) Class {
	var (
		expectLock bool   // last significant word was FOR, or a lock modifier
		prevWord   string // last word seen, for qualifier checks
		prevDot    bool   // last token was "."
	)

	for {
		text, kind := sc.next()

		switch kind {
		case tokEOF:
			return Read

		case tokWord:
			switch {
			case checkDML && dmlKeywords[text]:
				return Write

			// SELECT ... INTO newtbl creates a table.
			case text == "INTO":
				return Write

			case expectLock && (text == kwUpdate || text == kwShare):
				return Write

			case expectLock && lockModifiers[text]:
				// FOR NO KEY UPDATE: stay in the lock clause.

			case text == "FOR":
				expectLock = true

			case volatileFuncs[text] && sc.callFollows() && (!prevDot || prevWord == "PG_CATALOG"):
				return Write

			default:
				expectLock = false
			}

			prevWord = text
			prevDot = false

		case tokPunct:
			if text == ";" {
				// A terminator at the very end is ordinary. Anything after it
				// is a second statement this pass never looked at.
				if _, next := sc.next(); next == tokEOF {
					return Read
				}

				return Unknown
			}

			prevDot = text == "."
			expectLock = false

		case tokString, tokQuotedIdent, tokNumber:
			// A literal or a quoted identifier ends any pending lock clause and
			// cannot qualify a function name.
			prevDot = false
			expectLock = false

		default:
			return Unknown
		}
	}
}

// callFollows reports whether the next significant byte opens a call, without
// consuming anything. It is what keeps "SELECT nextval FROM t" a read while
// "SELECT nextval('s')" is not.
func (sc *scanner) callFollows() bool {
	saved := sc.pos

	sc.skipTrivia()

	open := sc.pos < len(sc.src) && sc.src[sc.pos] == '('
	sc.pos = saved

	return open
}

// skipTrivia consumes whitespace and comments, recording the first routing
// hint it finds.
func (sc *scanner) skipTrivia() {
	for sc.pos < len(sc.src) {
		char := sc.src[sc.pos]

		switch {
		case char == ' ' || char == '\t' || char == '\n' || char == '\r' || char == '\f' || char == '\v':
			sc.pos++

		case char == '-' && sc.peekNext() == '-':
			start := sc.pos
			sc.skipLineComment()
			sc.readHint(sc.src[start:sc.pos])

		case char == '/' && sc.peekNext() == '*':
			start := sc.pos
			sc.skipBlockComment()
			sc.readHint(sc.src[start:sc.pos])

		default:
			return
		}
	}
}

func (sc *scanner) readHint(comment string) {
	if sc.hintSet {
		return
	}

	lowered := strings.ToLower(comment)

	switch {
	case strings.Contains(lowered, hintRead):
		sc.hint, sc.hintSet = Read, true
	case strings.Contains(lowered, hintWrite):
		sc.hint, sc.hintSet = Write, true
	default:
		// Not a routing hint, just an ordinary comment.
	}
}

// next returns the following significant token.
func (sc *scanner) next() (string, tokenKind) {
	sc.skipTrivia()

	if sc.pos >= len(sc.src) {
		return "", tokEOF
	}

	char := sc.src[sc.pos]

	switch {
	case char == '\'':
		sc.skipQuoted('\'', false)

		return "", tokString

	case char == '"':
		start := sc.pos
		sc.skipQuoted('"', false)

		return sc.src[start:sc.pos], tokQuotedIdent

	case char == '$':
		if tag, ok := sc.dollarTag(); ok {
			sc.skipDollarQuoted(tag)

			return "", tokString
		}

		sc.pos++

		return "$", tokPunct

	case isDigit(char):
		start := sc.pos
		for sc.pos < len(sc.src) && (isDigit(sc.src[sc.pos]) || sc.src[sc.pos] == '.') {
			sc.pos++
		}

		return sc.src[start:sc.pos], tokNumber

	case isIdentStart(char):
		start := sc.pos
		for sc.pos < len(sc.src) && isIdentPart(sc.src[sc.pos]) {
			sc.pos++
		}

		word := asciiUpper(sc.src[start:sc.pos])

		// E'...', B'...', X'...' introduce a literal, not an identifier.
		if sc.pos < len(sc.src) && sc.src[sc.pos] == '\'' && stringPrefixes[word] {
			sc.skipQuoted('\'', word == "E")

			return "", tokString
		}

		return word, tokWord

	default:
		sc.pos++

		return sc.src[sc.pos-1 : sc.pos], tokPunct
	}
}

// peekNext returns the byte after the current one, or zero at the end.
func (sc *scanner) peekNext() byte {
	if sc.pos+1 >= len(sc.src) {
		return 0
	}

	return sc.src[sc.pos+1]
}

func (sc *scanner) skipLineComment() {
	for sc.pos < len(sc.src) && sc.src[sc.pos] != '\n' {
		sc.pos++
	}
}

// skipBlockComment honors nesting, which PostgreSQL allows and a regex
// cannot express.
func (sc *scanner) skipBlockComment() {
	sc.pos += 2
	depth := 1

	for sc.pos < len(sc.src) && depth > 0 {
		switch {
		case sc.src[sc.pos] == '/' && sc.peekNext() == '*':
			depth++
			sc.pos += 2
		case sc.src[sc.pos] == '*' && sc.peekNext() == '/':
			depth--
			sc.pos += 2
		default:
			sc.pos++
		}
	}
}

// skipQuoted consumes a quoted run, treating a doubled quote as an escaped
// one. Backslash escapes are honored only for E” literals: with
// standard_conforming_strings on, a backslash in a plain literal is a
// backslash, and treating it as an escape would run past the closing quote.
func (sc *scanner) skipQuoted(quote byte, backslashEscapes bool) {
	sc.pos++ // opening quote

	for sc.pos < len(sc.src) {
		char := sc.src[sc.pos]

		switch {
		case backslashEscapes && char == '\\':
			sc.pos += 2

		case char == quote:
			if sc.peekNext() == quote {
				sc.pos += 2

				continue
			}

			sc.pos++

			return

		default:
			sc.pos++
		}
	}
}

// dollarTag recognizes a $$ or $tag$ opener. A tag may not start with a digit,
// which is what keeps the $1 placeholder from looking like one.
func (sc *scanner) dollarTag() (string, bool) {
	end := sc.pos + 1

	if end < len(sc.src) && isDigit(sc.src[end]) {
		return "", false
	}

	for end < len(sc.src) && isIdentPart(sc.src[end]) {
		end++
	}

	if end >= len(sc.src) || sc.src[end] != '$' {
		return "", false
	}

	return sc.src[sc.pos : end+1], true
}

func (sc *scanner) skipDollarQuoted(tag string) {
	sc.pos += len(tag)

	closing := strings.Index(sc.src[sc.pos:], tag)
	if closing < 0 {
		sc.pos = len(sc.src)

		return
	}

	sc.pos += closing + len(tag)
}

func isDigit(char byte) bool { return char >= '0' && char <= '9' }

func isIdentStart(char byte) bool {
	return char == '_' ||
		(char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		char >= asciiMax // non-ASCII identifiers
}

// isIdentPart deliberately excludes '$'. PostgreSQL allows it inside an
// identifier, but including it here would let an identifier swallow the
// opening of an adjacent dollar-quoted body, and losing a dollar quote is the
// one mistake that lets a keyword inside a string literal reach the tables.
func isIdentPart(char byte) bool {
	return isIdentStart(char) || isDigit(char)
}

// asciiUpper uppercases without allocating when the word is already uppercase,
// which is the common case for keywords written in SQL style.
func asciiUpper(word string) string {
	needs := false

	for i := range len(word) {
		if word[i] >= 'a' && word[i] <= 'z' {
			needs = true

			break
		}
	}

	if !needs {
		return word
	}

	out := []byte(word)
	for i := range out {
		if out[i] >= 'a' && out[i] <= 'z' {
			out[i] -= 'a' - 'A'
		}
	}

	return string(out)
}
