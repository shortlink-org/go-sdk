package http_client

import "net/http"

// Middleware wraps a RoundTripper.
type Middleware func(next http.RoundTripper) http.RoundTripper

// RoundTripperFunc allows using a function as a RoundTripper.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// Chain builds a middleware pipeline.
func Chain(mw ...Middleware) Middleware {
	return func(final http.RoundTripper) http.RoundTripper {
		for i := len(mw) - 1; i >= 0; i-- {
			final = mw[i](final)
		}

		return final
	}
}
