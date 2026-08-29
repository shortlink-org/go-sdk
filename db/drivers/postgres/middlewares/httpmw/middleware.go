// Package httpmw carries a read-your-writes guarantee across an HTTP request
// boundary.
//
// A mutating response is stamped with the WAL position the caller's write
// reached; the caller's next request is gated on it, and reads the primary
// until a replica has replayed that far. Nothing is shared between processes,
// so it stays correct across pods and across a rolling deploy.
//
// It lives in the db module rather than in the http one because it needs the
// router, the token type and the routing context keys — three types that would
// have to be duplicated or abstracted away for no gain. It is still an ordinary
// net/http middleware and composes with chi, gorilla or the standard mux.
//
// The gRPC counterpart sits the other way round, in middlewares/grpcmw:
// there the transport module can satisfy a locally declared interface, which
// keeps the database's dependency graph out of the transport's.
package httpmw

import (
	"context"
	"net/http"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// Carrier names. The header is for API clients; the cookie is for browsers,
// which echo it without any client change.
const (
	// HeaderName carries the token both ways: set on a mutating response, read
	// on the requests that follow.
	//
	// Spelled in net/http's canonical form, because that is what goes on the
	// wire: Header.Set and Header.Get both canonicalize, so "X-Read-LSN" would
	// document a header that never appears.
	HeaderName = "X-Read-Lsn"

	// CookieName uses the __Host- prefix, which browsers only accept on a
	// secure origin with Path=/ and no Domain — so it cannot be set by a
	// sibling subdomain.
	CookieName = "__Host-rlsn"

	// unresolvedToken deliberately is not a valid WAL token. Receiving it
	// means a write happened but its position could not be captured, so the
	// read side pins the request to the primary instead of guessing.
	unresolvedToken = "unresolved"

	// The cookie's lifetime is short on purpose: the token is only useful until
	// the replica has replayed past it, and a stale one costs an unnecessary
	// primary read.
	cookieMaxAge = 30
)

// KeyFunc identifies the actor whose writes a later read must observe.
//
// It returns "" when the request has no identifiable actor, which disables the
// store path for that request. The value is used as a map key and is never
// logged, exported as a metric attribute, or sent to the client.
type KeyFunc func(*http.Request) string

// CarrierPolicy selects the client-visible transports for a watermark.
type CarrierPolicy uint8

const (
	// CarriersDefault uses X-Read-Lsn and is the zero-value policy.
	CarriersDefault CarrierPolicy = iota
	// CarriersNone disables client-visible transport while retaining request
	// scoping and an optional server-side Store.
	CarriersNone
	// CarriersHeader uses X-Read-Lsn only.
	CarriersHeader
	// CarriersCookie uses the secure __Host-rlsn cookie only.
	CarriersCookie
	// CarriersHeaderAndCookie uses both transports.
	CarriersHeaderAndCookie
)

// String implements fmt.Stringer.
func (p CarrierPolicy) String() string {
	switch p {
	case CarriersNone:
		return "none"
	case CarriersCookie:
		return "cookie"
	case CarriersHeaderAndCookie:
		return "header_and_cookie"
	default:
		return "header"
	}
}

// Config wires the middleware. Only Router is required.
type Config struct {
	// Router is the read-replica router the guarantee applies to.
	Router *replica.Router

	// Key identifies the actor, for the Store path. Leave it nil to rely on
	// the carrier alone.
	Key KeyFunc

	// Store persists a position per actor. Leave it nil — the carrier is
	// stateless and correct across pods; see postgres.MemoryWatermarks for why
	// an in-process store usually is not.
	Store replica.Watermarks

	// Carriers selects the client-visible transports. The zero value defaults
	// to the header. Choose a cookie policy only when the application
	// intentionally owns the browser cookie.
	Carriers CarrierPolicy
}

// KeyFromContextValue reads an actor id that upstream middleware placed in the
// request context under its own key type.
//
// The key has to be passed in rather than guessed. Context lookups compare
// dynamic types, so re-declaring a same-named key type here would silently
// never match the one the auth package uses. Wire it where both packages are
// already imported:
//
//	httpmw.KeyFromContextValue(session.ContextUserIDKey)
func KeyFromContextValue(key any) KeyFunc {
	return func(request *http.Request) string {
		id, ok := request.Context().Value(key).(string)
		if !ok {
			return ""
		}

		return id
	}
}

// New returns middleware that stamps a WAL watermark onto mutating responses,
// and holds subsequent reads by the same caller on the primary until a replica
// has replayed that far.
//
// Place it after authentication, which it needs for the actor, and outside
// any response-coalescing middleware: coalescing serves one response to
// several callers, so a follower's watermark is discarded by construction.
func New(cfg Config) func(http.Handler) http.Handler {
	middleware := boundaryMiddleware{
		router:   cfg.Router,
		carriers: newCarriers(cfg),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if middleware.router == nil {
				next.ServeHTTP(writer, request)

				return
			}

			if isMutating(request.Method) {
				if !middleware.carriers.enabled() {
					next.ServeHTTP(writer, request.WithContext(replica.WithTracker(request.Context())))

					return
				}

				middleware.serveMutating(next, writer, request)

				return
			}

			middleware.serveRead(next, writer, request)
		})
	}
}

type boundaryMiddleware struct {
	router   *replica.Router
	carriers responseCarriers
}

// responseCarriers owns all representations of a boundary watermark. This
// keeps header/cookie/store policy out of the request routing methods.
type responseCarriers struct {
	destinations carrierSet
	store        replica.Watermarks
	key          KeyFunc
}

type carrierSet uint8

const (
	headerCarrier carrierSet = 1 << iota
	cookieCarrier
)

func newCarriers(cfg Config) responseCarriers {
	destinations := headerCarrier
	switch cfg.Carriers {
	case CarriersNone:
		destinations = 0
	case CarriersCookie:
		destinations = cookieCarrier
	case CarriersHeaderAndCookie:
		destinations = headerCarrier | cookieCarrier
	case CarriersDefault, CarriersHeader:
		destinations = headerCarrier
	}

	return responseCarriers{
		destinations: destinations,
		store:        cfg.Store,
		key:          cfg.Key,
	}
}

func (c responseCarriers) enabled() bool {
	return c.destinations != 0 || (c.store != nil && c.key != nil)
}

func (c responseCarriers) carries(destination carrierSet) bool {
	return c.destinations&destination != 0
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// serveMutating scopes the request with a tracker and, if the handler actually
// wrote, stamps the resulting WAL position onto the response.
//
// The stamp has to happen before the first byte reaches the client, because
// headers are already on the wire after that — and a fast client can issue its
// follow-up read before any post-hoc capture would finish. The response writer
// is wrapped so the capture runs at exactly that moment, and only when there is
// something to capture: a mutating request that wrote nothing pays nothing.
func (m boundaryMiddleware) serveMutating(next http.Handler, writer http.ResponseWriter, request *http.Request) {
	ctx := replica.WithTracker(request.Context())

	stamper := &stampingWriter{
		ResponseWriter: writer,
		stamp: func() {
			tracker := replica.TrackerFromContext(ctx)
			if !tracker.Tainted() && tracker.Watermark() == 0 {
				return
			}

			token, err := m.router.Token(ctx)
			if err != nil {
				// Do not fail the response, but do carry an explicit unknown
				// marker. Carrying nothing would make the next request look like
				// a clean read and incorrectly allow it onto a replica.
				m.carriers.stamp(writer, request, ctx, unresolvedWatermark())

				return
			}

			m.carriers.stamp(writer, request, ctx, resolvedWatermark(token))
		},
	}

	next.ServeHTTP(stamper, request.WithContext(ctx))

	// A handler that returned without writing anything still wrote to the
	// database often enough to matter — 204 responses, for instance.
	stamper.flushStamp()
}

// serveRead scopes the request with whatever guarantee it arrived carrying.
func (m boundaryMiddleware) serveRead(next http.Handler, writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()

	carried := m.carriers.read(request)
	if carried.present() {
		next.ServeHTTP(writer, request.WithContext(carried.apply(ctx, m.router)))

		return
	}

	if m.carriers.store != nil && m.carriers.key != nil {
		if key := m.carriers.key(request); key != "" {
			position, found, err := m.carriers.store.Get(ctx, key)
			if err == nil && found {
				next.ServeHTTP(writer, request.WithContext(replica.WithWatermark(ctx, position)))

				return
			}
		}
	}

	// Nothing to observe: scope the request so its reads may use a replica.
	next.ServeHTTP(writer, request.WithContext(replica.WithTracker(ctx)))
}

type watermarkState uint8

const (
	watermarkAbsent watermarkState = iota
	watermarkResolved
	watermarkUnresolved
)

// carriedWatermark is a three-state value. In particular, unresolved is not
// collapsed into absent: it means a write is known to have happened and must
// conservatively pin the next read to the primary.
type carriedWatermark struct {
	state watermarkState
	token wal.Token
}

func resolvedWatermark(token wal.Token) carriedWatermark {
	return carriedWatermark{state: watermarkResolved, token: token}
}

func unresolvedWatermark() carriedWatermark {
	return carriedWatermark{state: watermarkUnresolved}
}

func (w carriedWatermark) present() bool { return w.state != watermarkAbsent }

func (w carriedWatermark) apply(ctx context.Context, router *replica.Router) context.Context {
	if w.state == watermarkResolved {
		// WithToken, not WithWatermark: a token this process cannot interpret
		// still says the caller has seen a write, and dropping it would serve
		// the stale read the token exists to prevent.
		return router.WithToken(ctx, w.token)
	}

	return replica.OnPrimary(ctx)
}

func parseWatermark(raw string) carriedWatermark {
	if raw == "" {
		return carriedWatermark{}
	}

	token, err := wal.ParseToken(raw)
	if err != nil {
		return unresolvedWatermark()
	}

	return resolvedWatermark(token)
}

func (c responseCarriers) read(request *http.Request) carriedWatermark {
	headerWatermark := carriedWatermark{}

	if c.carries(headerCarrier) {
		if raw := request.Header.Get(HeaderName); raw != "" {
			headerWatermark = parseWatermark(raw)
			if headerWatermark.state == watermarkResolved {
				return headerWatermark
			}
		}
	}

	if !c.carries(cookieCarrier) {
		return headerWatermark
	}

	cookie, err := request.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return headerWatermark
	}

	cookieWatermark := parseWatermark(cookie.Value)
	if cookieWatermark.state == watermarkResolved {
		return cookieWatermark
	}

	return unresolvedWatermark()
}

// stamp writes either a resolved token or an explicit unresolved marker to
// every configured carrier.
func (c responseCarriers) stamp(
	writer http.ResponseWriter,
	request *http.Request,
	ctx context.Context,
	watermark carriedWatermark,
) {
	raw := unresolvedToken
	position := wal.Unknown
	if watermark.state == watermarkResolved {
		raw = watermark.token.String()
		position = watermark.token.LSN
	}

	if c.carries(headerCarrier) {
		writer.Header().Set(HeaderName, raw)
	}

	if c.carries(cookieCarrier) {
		http.SetCookie(writer, &http.Cookie{
			Name:     CookieName,
			Value:    raw,
			Path:     "/",
			MaxAge:   cookieMaxAge,
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	if c.store != nil && c.key != nil {
		if key := c.key(request); key != "" {
			//nolint:errcheck // failure still leaves the carrier paths pinned
			_ = c.store.Set(ctx, key, position)
		}
	}
}

// stampingWriter runs stamp exactly once, immediately before the first byte of
// the response leaves.
//
// It forwards the optional interfaces a handler may reach for. Silently
// dropping Flusher would break streaming responses, and dropping Hijacker
// would break WebSocket upgrades — both failures that only show up in
// production, on the one endpoint nobody tested behind this middleware.
type stampingWriter struct {
	http.ResponseWriter

	stamp   func()
	stamped bool
}

func (w *stampingWriter) flushStamp() {
	if w.stamped {
		return
	}

	w.stamped = true
	w.stamp()
}

func (w *stampingWriter) WriteHeader(status int) {
	w.flushStamp()
	w.ResponseWriter.WriteHeader(status)
}

func (w *stampingWriter) Write(b []byte) (int, error) {
	w.flushStamp()

	return w.ResponseWriter.Write(b)
}

func (w *stampingWriter) Flush() {
	w.flushStamp()

	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer, which is
// how the standard library exposes deadlines and hijacking since Go 1.20.
func (w *stampingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
