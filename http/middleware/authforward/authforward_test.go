package authforward

import (
	"net/http"
	"net/http/httptest"
	"testing"

	grpc_authforward "github.com/shortlink-org/go-sdk/grpc/authforward"
	session_interceptor "github.com/shortlink-org/go-sdk/grpc/middleware/session"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestMiddleware_SetsAuthorization(t *testing.T) {
	t.Parallel()

	token := "Bearer abc123"
	handler := Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got := grpc_authforward.TokenFromContext(r.Context())
		require.Equal(t, token, got)

		gotAuth := session_interceptor.GetAuthorization(r.Context())
		require.Equal(t, token, gotAuth)

		outCtx := grpc_authforward.SetOutgoingToken(r.Context(), got)
		md, ok := metadata.FromOutgoingContext(outCtx)
		require.True(t, ok)
		require.Equal(t, []string{token}, md.Get("authorization"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", token)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestMiddleware_MultipleAuthorizationHeaders(t *testing.T) {
	t.Parallel()

	handler := Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		require.Empty(t, grpc_authforward.TokenFromContext(r.Context()))
		require.Empty(t, session_interceptor.GetAuthorization(r.Context()))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Authorization", "Bearer first")
	req.Header.Add("Authorization", "Bearer second")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}
