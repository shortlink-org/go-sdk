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

	// Header sets and reads the X-Read-LSN header. Default true.
	Header *bool

	// Cookie also sets and reads the __Host-rlsn cookie, so that browsers
	// echo the token unaided. Default false: a library should not put a cookie
	// on somebody's domain unless asked.
	Cookie bool
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
	header := cfg.Header == nil || *cfg.Header

	// Whether there is anywhere to put a watermark. With every carrier turned
	// off the middleware still earns its place — it scopes each request so its
	// reads may use a replica — but capturing a position nobody will read
	// would be a round trip spent on nothing.
	//
	// That configuration is the right one behind a load-balanced replica
	// endpoint, where a token cannot be trusted but the in-request taint still
	// is.
	carries := header || cfg.Cookie || (cfg.Store != nil && cfg.Key != nil)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if cfg.Router == nil {
				next.ServeHTTP(writer, request)

				return
			}

			if isMutating(request.Method) {
				if !carries {
					next.ServeHTTP(writer, request.WithContext(replica.WithTracker(request.Context())))

					return
				}

				serveMutating(cfg, header, next, writer, request)

				return
			}

			serveRead(cfg, header, next, writer, request)
		})
	}
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
func serveMutating(cfg Config, header bool, next http.Handler, writer http.ResponseWriter, request *http.Request) {
	ctx := replica.WithTracker(request.Context())

	stamper := &stampingWriter{
		ResponseWriter: writer,
		stamp: func() {
			tracker := replica.TrackerFromContext(ctx)
			if !tracker.Tainted() && tracker.Watermark() == 0 {
				return
			}

			token, err := cfg.Router.Token(ctx)
			if err != nil {
				// A failed capture must not fail the request: this is a
				// consistency optimization, and trading it for availability is
				// the wrong way round. The caller degrades to a primary read.
				return
			}

			if header {
				writer.Header().Set(HeaderName, token.String())
			}

			if cfg.Cookie {
				http.SetCookie(writer, &http.Cookie{
					Name:     CookieName,
					Value:    token.String(),
					Path:     "/",
					MaxAge:   cookieMaxAge,
					Secure:   true,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
			}

			if cfg.Store != nil && cfg.Key != nil {
				if key := cfg.Key(request); key != "" {
					//nolint:errcheck // a watermark that could not be stored degrades to a primary read
					_ = cfg.Store.Set(ctx, key, token.LSN)
				}
			}
		},
	}

	next.ServeHTTP(stamper, request.WithContext(ctx))

	// A handler that returned without writing anything still wrote to the
	// database often enough to matter — 204 responses, for instance.
	stamper.flushStamp()
}

// serveRead scopes the request with whatever guarantee it arrived carrying.
func serveRead(cfg Config, header bool, next http.Handler, writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()

	if token, ok := tokenFrom(request, header); ok {
		// WithToken, not WithWatermark: a token this process cannot interpret
		// still says the caller has seen a write, and dropping it would serve
		// the stale read the token exists to prevent.
		next.ServeHTTP(writer, request.WithContext(cfg.Router.WithToken(ctx, token)))

		return
	}

	if cfg.Store != nil && cfg.Key != nil {
		if key := cfg.Key(request); key != "" {
			position, found, err := cfg.Store.Get(ctx, key)
			if err == nil && found {
				next.ServeHTTP(writer, request.WithContext(replica.WithWatermark(ctx, position)))

				return
			}
		}
	}

	// Nothing to observe: scope the request so its reads may use a replica.
	next.ServeHTTP(writer, request.WithContext(replica.WithTracker(ctx)))
}

func tokenFrom(request *http.Request, header bool) (wal.Token, bool) {
	if header {
		if raw := request.Header.Get(HeaderName); raw != "" {
			token, err := wal.ParseToken(raw)
			if err == nil {
				return token, true
			}
		}
	}

	cookie, err := request.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return wal.Token{}, false
	}

	token, err := wal.ParseToken(cookie.Value)
	if err != nil {
		return wal.Token{}, false
	}

	return token, true
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
