package authforward

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/metadata"

	grpc_authforward "github.com/shortlink-org/go-sdk/grpc/authforward"
	session_interceptor "github.com/shortlink-org/go-sdk/grpc/middleware/session"
)

func TestMiddleware_SetsAuthorization(t *testing.T) {
	t.Parallel()

	token := "Bearer abc123"
	handler := Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got := grpc_authforward.TokenFromContext(r.Context())
		assert.Equal(t, token, got)

		gotAuth := session_interceptor.GetAuthorization(r.Context())
		assert.Equal(t, token, gotAuth)

		outCtx := grpc_authforward.SetOutgoingToken(r.Context(), got)
		md, ok := metadata.FromOutgoingContext(outCtx)
		assert.True(t, ok)
		assert.Equal(t, []string{token}, md.Get("authorization"))
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", token)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestMiddleware_MultipleAuthorizationHeaders(t *testing.T) {
	t.Parallel()

	handler := Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		assert.Empty(t, grpc_authforward.TokenFromContext(r.Context()))
		assert.Empty(t, session_interceptor.GetAuthorization(r.Context()))
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	req.Header.Add("Authorization", "Bearer first")
	req.Header.Add("Authorization", "Bearer second")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}
