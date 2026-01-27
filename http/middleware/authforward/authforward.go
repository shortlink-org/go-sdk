package authforward

import (
	"errors"
	"net/http"
	"strings"

	session_interceptor "github.com/shortlink-org/go-sdk/grpc/middleware/session"
)

// ErrMultipleAuthorizationValues indicates multiple Authorization headers.
var ErrMultipleAuthorizationValues = errors.New("multiple authorization headers")

// TokenExtractor extracts authorization tokens from HTTP requests.
type TokenExtractor interface {
	FromRequest(r *http.Request) (string, error)
}

// HeaderTokenExtractor extracts the Authorization header from requests.
type HeaderTokenExtractor struct{}

// FromRequest extracts a single Authorization header value.
func (HeaderTokenExtractor) FromRequest(r *http.Request) (string, error) {
	values := r.Header.Values("Authorization")
	if len(values) == 0 {
		return "", nil
	}

	if len(values) != 1 {
		return "", ErrMultipleAuthorizationValues
	}

	token := strings.TrimSpace(values[0])
	if token == "" {
		return "", nil
	}

	return token, nil
}

// Middleware forwards Authorization header into context for gRPC propagation.
func Middleware() func(http.Handler) http.Handler {
	return MiddlewareWithExtractor(HeaderTokenExtractor{})
}

// MiddlewareWithExtractor allows custom token extraction.
func MiddlewareWithExtractor(extractor TokenExtractor) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := extractor.FromRequest(r)
			if err == nil && token != "" {
				ctx := session_interceptor.WithAuthorization(r.Context(), token)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}
