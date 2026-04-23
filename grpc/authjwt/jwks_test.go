package authjwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rsaTestKeyBits = 2048

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.now = f.now.Add(d)
}

func jwksBody(t *testing.T, kid string, pub *rsa.PublicKey) []byte {
	t.Helper()

	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())

	body, err := json.Marshal(jwksResponse{
		Keys: []jwkKey{{
			Kty: "RSA",
			Use: "sig",
			Kid: kid,
			Alg: "RS256",
			N:   n,
			E:   e,
		}},
	})
	require.NoError(t, err)

	return body
}

func TestJWKSFetcher_CacheHit(t *testing.T) {
	t.Parallel()

	priv, err := rsa.GenerateKey(rand.Reader, rsaTestKeyBits)
	require.NoError(t, err)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)

		_, werr := w.Write(jwksBody(t, "kid-1", &priv.PublicKey))
		assert.NoError(t, werr)
	}))
	t.Cleanup(server.Close)

	clock := &fakeClock{now: time.Now()}
	fetcher := NewJWKSFetcher(JWKSConfig{
		URL:         server.URL,
		CacheTTL:    time.Minute,
		HTTPTimeout: time.Second,
		Clock:       clock,
	})

	key, err := fetcher.GetKey(context.Background(), "kid-1")
	require.NoError(t, err)
	require.Equal(t, priv.N, key.N)

	key, err = fetcher.GetKey(context.Background(), "kid-1")
	require.NoError(t, err)
	require.Equal(t, priv.N, key.N)

	require.Equal(t, int32(1), calls.Load())
}

func TestJWKSFetcher_Backoff(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	clock := &fakeClock{now: time.Now()}
	fetcher := NewJWKSFetcher(JWKSConfig{
		URL:         server.URL,
		CacheTTL:    time.Minute,
		HTTPTimeout: time.Second,
		BackoffMin:  time.Second,
		BackoffMax:  time.Second,
		Clock:       clock,
	})

	_, err := fetcher.GetKey(context.Background(), "kid-1")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnexpectedStatus)

	_, err = fetcher.GetKey(context.Background(), "kid-1")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrJWKSBackoff)

	clock.Advance(time.Second)
	_, err = fetcher.GetKey(context.Background(), "kid-1")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnexpectedStatus)

	require.GreaterOrEqual(t, calls.Load(), int32(2))
}
