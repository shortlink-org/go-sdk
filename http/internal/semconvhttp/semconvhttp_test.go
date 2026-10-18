package semconvhttp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSpanMethod(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.MethodGet, SpanMethod(http.MethodGet))
	require.Equal(t, http.MethodPatch, SpanMethod(http.MethodPatch))
	require.Equal(t, "HTTP", SpanMethod("PURGE"))
	require.Equal(t, "HTTP", SpanMethod("get"))
	require.Equal(t, "HTTP", SpanMethod(""))
}

func TestRoute(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/users/{id}", Route("GET /users/{id}"))
	require.Equal(t, "/users/{id}", Route("GET example.com/users/{id}"))
	require.Equal(t, "/users/{id}", Route("/users/{id}"))
	require.Empty(t, Route("GET"))
	require.Empty(t, Route(""))
}
