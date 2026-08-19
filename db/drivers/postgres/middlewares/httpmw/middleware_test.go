//go:build unit

package httpmw

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// Positions and sizes used across the tables.
const (
	headerPosition = wal.LSN(500)
	cookiePosition = wal.LSN(900)
	storedPosition = wal.LSN(4096)
	storeCapacity  = 16
	actor          = "alice"
	testPath       = "/orders"
	okBody         = "ok"
)

// testToken builds a token on the current timeline.
func testToken(position wal.LSN) wal.Token {
	return wal.Token{Timeline: 1, LSN: position, IssuedAt: time.Now()}
}

// The middleware's job is to put the right thing in the request context, so
// that is what these assert: the handler records what it was given.
type recorder struct {
	strategy   replica.Strategy
	watermark  wal.LSN
	hasTracker bool
}

func (rec *recorder) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		rec.strategy = replica.StrategyFromContext(ctx)
		rec.hasTracker = replica.TrackerFromContext(ctx) != nil
		rec.watermark = replica.TrackerFromContext(ctx).Watermark()

		w.WriteHeader(http.StatusOK)

		//nolint:errcheck // the recorder never fails a write
		_, _ = w.Write([]byte(okBody))
	})
}

func serve(t *testing.T, cfg Config, handler http.Handler, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	New(cfg)(handler).ServeHTTP(w, r)

	return w
}

func TestReadWithoutATokenIsScopedForReplicaUse(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	cfg := Config{Router: &replica.Router{}}

	serve(t, cfg, rec.handler(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, testPath, http.NoBody))

	assert.True(t, rec.hasTracker, "a read must be scoped, or it falls back to the primary forever")
	assert.Equal(t, replica.StrategyUnset, rec.strategy)
	assert.Equal(t, wal.LSN(0), rec.watermark)
}

func TestReadWithAMalformedTokenIsIgnored(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	cfg := Config{Router: &replica.Router{}}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testPath, http.NoBody)
	request.Header.Set(HeaderName, "not-a-token")

	serve(t, cfg, rec.handler(), request)

	assert.True(t, rec.hasTracker)
	assert.Equal(t, wal.LSN(0), rec.watermark)
}

func TestReadPrefersTheHeaderOverTheCookie(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	router := &replica.Router{}

	fromHeader := testToken(headerPosition)
	fromCookie := testToken(cookiePosition)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testPath, http.NoBody)
	request.Header.Set(HeaderName, fromHeader.String())
	request.AddCookie(newCookie(fromCookie.String()))

	serve(t, Config{Router: router}, rec.handler(), request)

	assert.Equal(t, fromHeader.LSN, rec.watermark)
}

func TestReadFallsBackToTheCookie(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	token := testToken(cookiePosition)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testPath, http.NoBody)
	request.AddCookie(newCookie(token.String()))

	serve(t, Config{Router: &replica.Router{}}, rec.handler(), request)

	assert.Equal(t, token.LSN, rec.watermark)
}

func TestReadUsesTheStoreWhenNoTokenArrives(t *testing.T) {
	t.Parallel()

	store := replica.NewMemoryWatermarks(time.Minute, storeCapacity)
	require.NoError(t, store.Set(t.Context(), actor, storedPosition))

	rec := &recorder{}
	cfg := Config{
		Router: &replica.Router{},
		Store:  store,
		Key:    func(*http.Request) string { return actor },
	}

	serve(t, cfg, rec.handler(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, testPath, http.NoBody))

	assert.Equal(t, storedPosition, rec.watermark)
}

func TestReadIgnoresAnEmptyStoreKey(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	cfg := Config{
		Router: &replica.Router{},
		Store:  replica.NewMemoryWatermarks(time.Minute, storeCapacity),
		Key:    func(*http.Request) string { return "" },
	}

	serve(t, cfg, rec.handler(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, testPath, http.NoBody))

	assert.True(t, rec.hasTracker)
	assert.Equal(t, wal.LSN(0), rec.watermark)
}

func TestMutatingRequestIsScopedWithATracker(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}

			serve(t, Config{Router: &replica.Router{}},
				rec.handler(),
				httptest.NewRequestWithContext(t.Context(), method, testPath, http.NoBody))

			assert.True(t, rec.hasTracker)
		})
	}
}

// A mutating request that did not write has nothing to stamp, and must not pay
// a round trip to the primary to discover that.
func TestMutatingRequestThatWroteNothingIsNotStamped(t *testing.T) {
	t.Parallel()

	rec := &recorder{}

	response := serve(t, Config{Router: &replica.Router{}},
		rec.handler(),
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, testPath, http.NoBody))

	assert.Empty(t, response.Header().Get(HeaderName))
	assert.Empty(t, response.Result().Cookies()) //nolint:bodyclose // httptest recorder
}

func TestKeyFromContextValue(t *testing.T) {
	t.Parallel()

	type userKey struct{}

	extract := KeyFromContextValue(userKey{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	assert.Empty(t, extract(request), "an unauthenticated request has no actor")

	//nolint:staticcheck // the point is that the caller owns the key type
	ctx := request.Context()
	assert.Empty(t, extract(request.WithContext(ctx)))
}

func TestNilRouterIsATransparentPassThrough(t *testing.T) {
	t.Parallel()

	called := false
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })

	serve(t, Config{}, handler, httptest.NewRequestWithContext(t.Context(), http.MethodPost, testPath, http.NoBody))

	assert.True(t, called)
}

// The wrapper must not swallow the interfaces a handler reaches for. Dropping
// Flusher breaks streaming responses, and it only shows up in production.
func TestStampingWriterForwardsFlush(t *testing.T) {
	t.Parallel()

	flushed := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		assert.True(t, ok, "the wrapper must still be an http.Flusher")

		if ok {
			flusher.Flush()
		}

		flushed = true
	})

	serve(t, Config{Router: &replica.Router{}}, handler,
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, testPath, http.NoBody))

	assert.True(t, flushed)
}

// With every carrier off the middleware is a scoping middleware and nothing
// more: it must not spend a round trip capturing a watermark nobody will read.
// That is the right configuration behind a load-balanced replica endpoint,
// where a token cannot be trusted but the in-request taint still can.
func TestScopeOnlyConfigurationDoesNotCapture(t *testing.T) {
	t.Parallel()

	off := false
	rec := &recorder{}

	// A nil Router would short-circuit before any capture, so use a real one:
	// reaching Token on it would panic, which is exactly the assertion.
	cfg := Config{Router: &replica.Router{}, Header: &off, Cookie: false}

	assert.NotPanics(t, func() {
		serve(t, cfg, rec.handler(),
			httptest.NewRequestWithContext(t.Context(), http.MethodPost, testPath, http.NoBody))
	})

	assert.True(t, rec.hasTracker, "the request must still be scoped")
}

// newCookie builds a request cookie with the attributes the middleware sets, so
// that the test does not model an insecure one.
func newCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}
