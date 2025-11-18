package http_client

import (
	"net/http"

	"github.com/shortlink-org/go-sdk/http/client/internal/types"
)

// Middleware is an alias for types.Middleware for backward compatibility.
type Middleware = types.Middleware

// RoundTripperFunc is an alias for types.RoundTripperFunc for backward compatibility.
type RoundTripperFunc = types.RoundTripperFunc

// Chain builds a middleware pipeline.
func Chain(mw ...types.Middleware) types.Middleware {
	return func(final http.RoundTripper) http.RoundTripper {
		for i := len(mw) - 1; i >= 0; i-- {
			final = mw[i](final)
		}

		return final
	}
}
