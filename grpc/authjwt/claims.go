package authjwt

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type claimsContextKey struct{}

var claimsKey = claimsContextKey{}

// Claims represents validated JWT claims.
// Extends RegisteredClaims with custom fields from Oathkeeper id_token mutator.
//
//nolint:tagliatelle // Oathkeeper sends claims in snake_case format
type Claims struct {
	jwt.RegisteredClaims

	// Custom claims from Oathkeeper
	Email      string         `json:"email,omitempty"`
	Name       string         `json:"name,omitempty"`
	IdentityID string         `json:"identity_id,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// WithClaims stores validated claims in context.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// ClaimsFromContext retrieves validated claims from context.
// Returns nil if not present.
func ClaimsFromContext(ctx context.Context) *Claims {
	if v, ok := ctx.Value(claimsKey).(*Claims); ok {
		return v
	}

	return nil
}

// GetSubject returns the subject (user ID) from context.
// Returns empty string if claims not present.
func GetSubject(ctx context.Context) string {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}

	return claims.Subject
}

// GetEmail returns the email from context.
// Returns empty string if claims not present.
func GetEmail(ctx context.Context) string {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}

	return claims.Email
}

// IsAuthenticated returns true if context has valid claims.
func IsAuthenticated(ctx context.Context) bool {
	return ClaimsFromContext(ctx) != nil
}

// GetExpiresAt returns the token expiration time.
// Returns zero time if claims not present.
func GetExpiresAt(ctx context.Context) time.Time {
	claims := ClaimsFromContext(ctx)
	if claims == nil || claims.ExpiresAt == nil {
		return time.Time{}
	}

	return claims.ExpiresAt.Time
}

// GetIssuedAt returns the token issue time.
// Returns zero time if claims not present.
func GetIssuedAt(ctx context.Context) time.Time {
	claims := ClaimsFromContext(ctx)
	if claims == nil || claims.IssuedAt == nil {
		return time.Time{}
	}

	return claims.IssuedAt.Time
}
