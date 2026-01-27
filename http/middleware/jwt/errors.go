package jwt_middleware

import "errors"

var (
	// ErrInvalidToken is returned when the JWT token is malformed or invalid.
	ErrInvalidToken = errors.New("invalid token")

	// ErrTokenExpired is returned when the JWT token has expired.
	ErrTokenExpired = errors.New("token expired")

	// ErrMissingSubject is returned when the JWT token is missing the subject claim.
	ErrMissingSubject = errors.New("missing subject in token")
)
