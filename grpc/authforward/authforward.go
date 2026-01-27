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
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	values := md.Get(MetadataKey)
	if len(values) == 0 {
		return ""
	}

	// Take only the first value to prevent header injection
	return strings.TrimSpace(values[0])
}

// TokenFromOutgoingMetadata extracts the authorization token from outgoing gRPC metadata.
// Returns empty string if not present.
func TokenFromOutgoingMetadata(ctx context.Context) string {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return ""
	}

	values := md.Get(MetadataKey)
	if len(values) == 0 {
		return ""
	}

	return strings.TrimSpace(values[0])
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
	if !strings.HasPrefix(auth, BearerPrefix) {
		return ""
	}

	return strings.TrimPrefix(auth, BearerPrefix)
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
