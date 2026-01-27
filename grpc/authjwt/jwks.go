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
)

const (
	// DefaultJWKSCacheTTL is the default cache duration for JWKS keys.
	DefaultJWKSCacheTTL = time.Hour
	// DefaultJWKSHTTPTimeout is the default HTTP timeout for JWKS fetch.
	DefaultJWKSHTTPTimeout = 10 * time.Second
	// maxJWKSBodySize is the maximum size of JWKS response body (1MB).
	maxJWKSBodySize = 1 << 20
	// exponentBitShift is used for RSA exponent parsing.
	exponentBitShift = 8
)

// JWKSFetcher fetches and caches JWKS (JSON Web Key Set) from a remote URL.
// It is concurrency-safe and handles automatic refresh on cache miss.
type JWKSFetcher struct {
	url        string
	httpClient *http.Client
	cacheTTL   time.Duration

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time

	// For preventing thundering herd on cache miss
	fetchMu   sync.Mutex
	fetching  bool
	fetchCond *sync.Cond
}

// JWKSConfig configures the JWKS fetcher.
type JWKSConfig struct {
	// URL is the JWKS endpoint URL
	URL string
	// CacheTTL is how long to cache keys before refresh (default: 1 hour)
	CacheTTL time.Duration
	// HTTPTimeout is the timeout for HTTP requests (default: 10 seconds)
	HTTPTimeout time.Duration
}

// NewJWKSFetcher creates a new JWKS fetcher.
func NewJWKSFetcher(cfg JWKSConfig) *JWKSFetcher {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = DefaultJWKSCacheTTL
	}

	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = DefaultJWKSHTTPTimeout
	}

	fetcher := &JWKSFetcher{
		url:      cfg.URL,
		cacheTTL: cfg.CacheTTL,
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
func (fetcher *JWKSFetcher) GetKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	// Try cache first
	fetcher.mu.RLock()
	key, found := fetcher.keys[kid]
	needsRefresh := time.Since(fetcher.fetchedAt) > fetcher.cacheTTL
	fetcher.mu.RUnlock()

	if found && !needsRefresh {
		return key, nil
	}

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
func (fetcher *JWKSFetcher) KeyFunc(ctx context.Context) jwt.Keyfunc {
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
func (fetcher *JWKSFetcher) Close() error {
	return nil
}

// refresh fetches the JWKS from the remote URL.
// Uses a condition variable to prevent thundering herd.
func (fetcher *JWKSFetcher) refresh(ctx context.Context) error {
	fetcher.fetchMu.Lock()

	// If another goroutine is already fetching, wait for it
	for fetcher.fetching {
		fetcher.fetchCond.Wait()
	}

	// Check if cache was refreshed while we were waiting
	fetcher.mu.RLock()

	if time.Since(fetcher.fetchedAt) < fetcher.cacheTTL {
		fetcher.mu.RUnlock()
		fetcher.fetchMu.Unlock()

		return nil
	}

	fetcher.mu.RUnlock()

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

func (fetcher *JWKSFetcher) doFetch(ctx context.Context) error {
	body, err := fetcher.fetchJWKSBody(ctx)
	if err != nil {
		return err
	}

	keys, err := fetcher.parseJWKS(body)
	if err != nil {
		return err
	}

	fetcher.mu.Lock()
	fetcher.keys = keys
	fetcher.fetchedAt = time.Now()
	fetcher.mu.Unlock()

	return nil
}

func (fetcher *JWKSFetcher) fetchJWKSBody(ctx context.Context) ([]byte, error) {
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

func (fetcher *JWKSFetcher) parseJWKS(body []byte) (map[string]*rsa.PublicKey, error) {
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
