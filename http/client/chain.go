package http_client

import (
	"net/http"
	"slices"

	"github.com/shortlink-org/go-sdk/http/client/internal/types"
)

// Middleware is an alias for types.Middleware for backward compatibility.
type Middleware = types.Middleware

// RoundTripperFunc is an alias for types.RoundTripperFunc for backward compatibility.
type RoundTripperFunc = types.RoundTripperFunc

// Chain builds a middleware pipeline.
func Chain(mw ...types.Middleware) types.Middleware {
	return func(final http.RoundTripper) http.RoundTripper {
		for _, middleware := range slices.Backward(mw) {
			final = middleware(final)
		}

		return final
	}
}
