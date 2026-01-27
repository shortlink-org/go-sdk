package session

import (
	"context"
)

type Session string

const (
	// contextClaimsKey is the key used to store JWT claims in the context.
	contextClaimsKey = Session("jwt-claims")

	// ContextUserIDKey is the key used to store the user id in the context.
	ContextUserIDKey = Session("user-id")
)

// Claims represents JWT claims from Oathkeeper id_token mutator.
// These claims are set by Oathkeeper after validating the session with Kratos.
type Claims struct {
	// Subject is the user ID (from Kratos identity)
	Subject string `json:"sub"`
	// Email from identity traits
	Email string `json:"email"`
	// Name from identity traits
	Name string `json:"name"`
	// IdentityID is the Kratos identity ID
	IdentityID string `json:"identity_id"`
	// SessionID is the Kratos session ID
	SessionID string `json:"session_id"`
	// Metadata from identity metadata_public
	Metadata map[string]any `json:"metadata"`
	// Issuer of the token
	Issuer string `json:"iss"`
	// IssuedAt timestamp
	IssuedAt int64 `json:"iat"`
	// ExpiresAt timestamp
	ExpiresAt int64 `json:"exp"`
}

// String returns the string representation of the session.
func (s Session) String() string {
	return string(s)
}

// WithClaims stores JWT claims in the context.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, contextClaimsKey, claims)
}

// GetClaims retrieves JWT claims from the context.
func GetClaims(ctx context.Context) (*Claims, error) {
	claims := ctx.Value(contextClaimsKey)
	if claims == nil {
		return nil, ErrSessionNotFound
	}

	if c, ok := claims.(*Claims); ok {
		return c, nil
	}

	return nil, ErrSessionNotFound
}

// WithUserID stores the user ID in the context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ContextUserIDKey, userID)
}

// GetUserID retrieves the user ID from the context.
func GetUserID(ctx context.Context) (string, error) {
	userID := ctx.Value(ContextUserIDKey)
	if userID == nil {
		return "", ErrUserIDNotFound
	}

	if uid, ok := userID.(string); ok {
		return uid, nil
	}

	return "", ErrUserIDNotFound
}
