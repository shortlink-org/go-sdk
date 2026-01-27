package authjwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.now = f.now.Add(d)
}

func jwksBody(kid string, pub *rsa.PublicKey) []byte {
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())

	body, _ := json.Marshal(jwksResponse{
		Keys: []jwkKey{{
			Kty: "RSA",
			Use: "sig",
			Kid: kid,
			Alg: "RS256",
			N:   n,
			E:   e,
		}},
	})

	return body
}

func TestJWKSFetcher_CacheHit(t *testing.T) {
	t.Parallel()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksBody("kid-1", &priv.PublicKey))
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
	require.Equal(t, priv.PublicKey.N, key.N)

	key, err = fetcher.GetKey(context.Background(), "kid-1")
	require.NoError(t, err)
	require.Equal(t, priv.PublicKey.N, key.N)

	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestJWKSFetcher_Backoff(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
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
	require.True(t, errors.Is(err, ErrUnexpectedStatus))

	_, err = fetcher.GetKey(context.Background(), "kid-1")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrJWKSBackoff))

	clock.Advance(time.Second)
	_, err = fetcher.GetKey(context.Background(), "kid-1")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUnexpectedStatus))

	require.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(2))
}
