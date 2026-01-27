// Package authforward provides gRPC interceptors for capturing and forwarding
// Authorization headers (JWT tokens) across service boundaries.
//
// This package implements the "token relay" pattern where:
// 1. Server interceptor captures incoming Authorization from metadata
// 2. Token is stored in context
// 3. Client interceptor forwards token to outgoing metadata
//
// Security considerations:
// - This package does NOT validate tokens, only forwards them
// - Use with authjwt package for validation at service boundaries
// - Only use within trusted mesh boundaries (mTLS recommended)
// - Tokens should have short TTL (15 min recommended)
// - Validate audience claim to prevent token confusion attacks
package authforward

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/metadata"
)

// MetadataKey is the gRPC metadata key for authorization.
// Must be lowercase for gRPC metadata compliance.
const MetadataKey = "authorization"

// BearerPrefix is the expected prefix for Bearer tokens.
const BearerPrefix = "Bearer "

type contextKey struct{}

// authContextKey is the context key for storing the authorization token.
var authContextKey = contextKey{}

// ErrMultipleAuthorizationValues indicates multiple authorization values in metadata.
var ErrMultipleAuthorizationValues = errors.New("multiple authorization values")

// TokenExtractor extracts authorization tokens from gRPC metadata.
type TokenExtractor interface {
	FromIncomingMetadata(ctx context.Context) (string, error)
	FromOutgoingMetadata(ctx context.Context) (string, error)
}

// MetadataTokenExtractor extracts tokens from gRPC metadata.
type MetadataTokenExtractor struct{}

// FromIncomingMetadata extracts token from incoming metadata.
func (MetadataTokenExtractor) FromIncomingMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", nil
	}

	return tokenFromMetadata(md)
}

// FromOutgoingMetadata extracts token from outgoing metadata.
func (MetadataTokenExtractor) FromOutgoingMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return "", nil
	}

	return tokenFromMetadata(md)
}

// WithToken stores the authorization token in context.
// The token should include the "Bearer " prefix.
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, authContextKey, token)
}

// TokenFromContext retrieves the authorization token from context.
// Returns empty string if not present.
func TokenFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(authContextKey).(string); ok {
		return v
	}

	return ""
}

// TokenFromIncomingMetadata extracts the authorization token from incoming gRPC metadata.
// Returns empty string if not present or invalid.
func TokenFromIncomingMetadata(ctx context.Context) string {
	token, _ := MetadataTokenExtractor{}.FromIncomingMetadata(ctx)
	return token
}

// TokenFromOutgoingMetadata extracts the authorization token from outgoing gRPC metadata.
// Returns empty string if not present.
func TokenFromOutgoingMetadata(ctx context.Context) string {
	token, _ := MetadataTokenExtractor{}.FromOutgoingMetadata(ctx)
	return token
}

// SetOutgoingToken sets (not appends) the authorization token in outgoing metadata.
// This prevents accumulation of multiple authorization values across hops.
func SetOutgoingToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}

	// Get existing metadata or create new
	outgoingMD, exists := metadata.FromOutgoingContext(ctx)
	if !exists {
		outgoingMD = metadata.MD{}
	}

	// Create a copy to avoid mutating the original
	newMD := outgoingMD.Copy()

	// Set (replace) the authorization value - prevents accumulation
	newMD.Set(MetadataKey, token)

	return metadata.NewOutgoingContext(ctx, newMD)
}

// ExtractBearerToken extracts the token value from a "Bearer <token>" string.
// Returns empty string if the format is invalid.
func ExtractBearerToken(auth string) string {
	if len(auth) < len(BearerPrefix) {
		return ""
	}

	if !strings.EqualFold(auth[:len(BearerPrefix)], BearerPrefix) {
		return ""
	}

	return strings.TrimSpace(auth[len(BearerPrefix):])
}

func tokenFromMetadata(md metadata.MD) (string, error) {
	values := md.Get(MetadataKey)
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

// FormatBearerToken formats a token as "Bearer <token>".
func FormatBearerToken(token string) string {
	if token == "" {
		return ""
	}

	if strings.HasPrefix(token, BearerPrefix) {
		return token
	}

	return BearerPrefix + token
}
