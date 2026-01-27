package authjwt

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// DefaultJWKSCacheTTL is the default cache duration for JWKS keys.
	DefaultJWKSCacheTTL = time.Hour
	// DefaultJWKSHTTPTimeout is the default HTTP timeout for JWKS fetch.
	DefaultJWKSHTTPTimeout = 10 * time.Second
	// DefaultJWKSBackoffMin is the minimum backoff after JWKS fetch failures.
	DefaultJWKSBackoffMin = 500 * time.Millisecond
	// DefaultJWKSBackoffMax is the maximum backoff after JWKS fetch failures.
	DefaultJWKSBackoffMax = 30 * time.Second
	// maxJWKSBodySize is the maximum size of JWKS response body (1MB).
	maxJWKSBodySize = 1 << 20
	// exponentBitShift is used for RSA exponent parsing.
	exponentBitShift = 8
)

var (
	jwksCacheAccessTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jwks_cache_access_total",
			Help: "Total JWKS cache accesses.",
		},
		[]string{"result"},
	)
	jwksFetchTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jwks_fetch_total",
			Help: "Total JWKS fetch attempts.",
		},
		[]string{"outcome"},
	)
	jwksFetchSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "jwks_fetch_seconds",
			Help:    "Time spent fetching JWKS.",
			Buckets: prometheus.DefBuckets,
		},
	)
)

// JWKSFetcher fetches and caches JWKS keys for validation.
type JWKSFetcher interface {
	KeyFunc(ctx context.Context) jwt.Keyfunc
	GetKey(ctx context.Context, kid string) (*rsa.PublicKey, error)
	Close() error
}

// jwksFetcher fetches and caches JWKS (JSON Web Key Set) from a remote URL.
// It is concurrency-safe and handles automatic refresh on cache miss.
type jwksFetcher struct {
	url        string
	httpClient *http.Client
	cacheTTL   time.Duration
	backoffMin time.Duration
	backoffMax time.Duration
	clock      Clock

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time

	// For preventing thundering herd on cache miss
	fetchMu   sync.Mutex
	fetching  bool
	fetchCond *sync.Cond

	// backoff state (guarded by fetchMu)
	backoff   time.Duration
	nextRetry time.Time
}

// JWKSConfig configures the JWKS fetcher.
type JWKSConfig struct {
	// URL is the JWKS endpoint URL
	URL string
	// CacheTTL is how long to cache keys before refresh (default: 1 hour)
	CacheTTL time.Duration
	// HTTPTimeout is the timeout for HTTP requests (default: 10 seconds)
	HTTPTimeout time.Duration
	// BackoffMin is the minimum backoff after a failed JWKS fetch (default: 500ms)
	BackoffMin time.Duration
	// BackoffMax is the maximum backoff after a failed JWKS fetch (default: 30s)
	BackoffMax time.Duration
	// Clock overrides the time source (default: real clock)
	Clock Clock
}

// NewJWKSFetcher creates a new JWKS fetcher.
func NewJWKSFetcher(cfg JWKSConfig) JWKSFetcher {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = DefaultJWKSCacheTTL
	}

	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = DefaultJWKSHTTPTimeout
	}

	if cfg.BackoffMin == 0 {
		cfg.BackoffMin = DefaultJWKSBackoffMin
	}

	if cfg.BackoffMax == 0 {
		cfg.BackoffMax = DefaultJWKSBackoffMax
	}

	if cfg.BackoffMax < cfg.BackoffMin {
		cfg.BackoffMax = cfg.BackoffMin
	}

	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}

	fetcher := &jwksFetcher{
		url:        cfg.URL,
		cacheTTL:   cfg.CacheTTL,
		backoffMin: cfg.BackoffMin,
		backoffMax: cfg.BackoffMax,
		clock:      cfg.Clock,
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
		keys: make(map[string]*rsa.PublicKey),
	}
	fetcher.fetchCond = sync.NewCond(&fetcher.fetchMu)

	return fetcher
}

// GetKey retrieves a public key by key ID (kid).
// If the key is not in cache, it will attempt to refresh from the JWKS URL.
func (fetcher *jwksFetcher) GetKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	// Try cache first
	fetcher.mu.RLock()
	key, found := fetcher.keys[kid]
	needsRefresh := fetcher.clock.Now().Sub(fetcher.fetchedAt) > fetcher.cacheTTL
	fetcher.mu.RUnlock()

	if found && !needsRefresh {
		jwksCacheAccessTotal.WithLabelValues("hit").Inc()
		return key, nil
	}

	jwksCacheAccessTotal.WithLabelValues("miss").Inc()

	// Cache miss or expired - refresh
	err := fetcher.refresh(ctx)
	if err != nil {
		// If refresh fails but we have a cached key, use it
		if found {
			return key, nil
		}

		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	// Try again after refresh
	fetcher.mu.RLock()
	key, found = fetcher.keys[kid]
	fetcher.mu.RUnlock()

	if !found {
		return nil, fmt.Errorf("%w: kid=%q", ErrKeyNotFound, kid)
	}

	return key, nil
}

// KeyFunc returns a jwt.Keyfunc for use with jwt.Parse.
func (fetcher *jwksFetcher) KeyFunc(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		// Validate algorithm
		if _, isRSA := token.Method.(*jwt.SigningMethodRSA); !isRSA {
			return nil, fmt.Errorf("%w: %v", ErrUnexpectedSignMethod, token.Header["alg"])
		}

		// Get kid from header
		kid, hasKid := token.Header["kid"].(string)
		if !hasKid {
			return nil, ErrMissingKid
		}

		return fetcher.GetKey(ctx, kid)
	}
}

// Close releases resources. Currently a no-op but included for future use.
func (fetcher *jwksFetcher) Close() error {
	return nil
}

// refresh fetches the JWKS from the remote URL.
// Uses a condition variable to prevent thundering herd.
func (fetcher *jwksFetcher) refresh(ctx context.Context) error {
	fetcher.fetchMu.Lock()

	// If another goroutine is already fetching, wait for it
	for fetcher.fetching {
		fetcher.fetchCond.Wait()
	}

	// Check if cache was refreshed while we were waiting
	fetcher.mu.RLock()

	if fetcher.clock.Now().Sub(fetcher.fetchedAt) < fetcher.cacheTTL {
		fetcher.mu.RUnlock()
		fetcher.fetchMu.Unlock()

		return nil
	}

	fetcher.mu.RUnlock()

	// Respect backoff after recent failures
	if !fetcher.nextRetry.IsZero() && fetcher.clock.Now().Before(fetcher.nextRetry) {
		fetcher.fetchMu.Unlock()
		return ErrJWKSBackoff
	}

	// We're the one doing the fetch
	fetcher.fetching = true
	fetcher.fetchMu.Unlock()

	// Ensure we signal completion and release lock
	defer func() {
		fetcher.fetchMu.Lock()
		fetcher.fetching = false
		fetcher.fetchCond.Broadcast()
		fetcher.fetchMu.Unlock()
	}()

	return fetcher.doFetch(ctx)
}

func (fetcher *jwksFetcher) doFetch(ctx context.Context) error {
	start := fetcher.clock.Now()

	body, err := fetcher.fetchJWKSBody(ctx)
	if err != nil {
		fetcher.recordFetchFailure(time.Since(start))
		return err
	}

	keys, err := fetcher.parseJWKS(body)
	if err != nil {
		fetcher.recordFetchFailure(time.Since(start))
		return err
	}

	fetcher.mu.Lock()
	fetcher.keys = keys
	fetcher.fetchedAt = fetcher.clock.Now()
	fetcher.mu.Unlock()

	fetcher.recordFetchSuccess(fetcher.clock.Now().Sub(start))

	return nil
}

func (fetcher *jwksFetcher) recordFetchSuccess(duration time.Duration) {
	jwksFetchTotal.WithLabelValues("success").Inc()
	jwksFetchSeconds.Observe(duration.Seconds())

	fetcher.fetchMu.Lock()
	fetcher.backoff = 0
	fetcher.nextRetry = time.Time{}
	fetcher.fetchMu.Unlock()
}

func (fetcher *jwksFetcher) recordFetchFailure(duration time.Duration) {
	jwksFetchTotal.WithLabelValues("failure").Inc()
	jwksFetchSeconds.Observe(duration.Seconds())

	fetcher.fetchMu.Lock()
	if fetcher.backoff == 0 {
		fetcher.backoff = fetcher.backoffMin
	} else {
		fetcher.backoff *= 2
		if fetcher.backoff > fetcher.backoffMax {
			fetcher.backoff = fetcher.backoffMax
		}
	}
	fetcher.nextRetry = fetcher.clock.Now().Add(fetcher.backoff)
	fetcher.fetchMu.Unlock()
}

func (fetcher *jwksFetcher) fetchJWKSBody(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetcher.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := fetcher.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBodySize))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return body, nil
}

func (fetcher *jwksFetcher) parseJWKS(body []byte) (map[string]*rsa.PublicKey, error) {
	var jwks jwksResponse

	err := json.Unmarshal(body, &jwks)
	if err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)

	for _, key := range jwks.Keys {
		if key.Kty != "RSA" {
			continue
		}

		if key.Use != "" && key.Use != "sig" {
			continue
		}

		pubKey, err := parseRSAPublicKey(key)
		if err != nil {
			continue // Skip invalid keys
		}

		keys[key.Kid] = pubKey
	}

	if len(keys) == 0 {
		return nil, ErrNoValidKeys
	}

	return keys, nil
}

// JWKS response structures.
type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func parseRSAPublicKey(key jwkKey) (*rsa.PublicKey, error) {
	modulusBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}

	exponentBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}

	modulus := new(big.Int).SetBytes(modulusBytes)

	exponent := 0
	for _, b := range exponentBytes {
		exponent = exponent<<exponentBitShift + int(b)
	}

	return &rsa.PublicKey{N: modulus, E: exponent}, nil
}
