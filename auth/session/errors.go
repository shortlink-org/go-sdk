// Package session stores JWT session claims and user identifiers in context.
package session

import "errors"

var (
	// ErrSessionNotFound is returned when JWT claims are absent or have the wrong type.
	ErrSessionNotFound = errors.New("session not found")
	// ErrMetadataNotFound is returned when expected metadata is missing from claims.
	ErrMetadataNotFound = errors.New("metadata not found")
	// ErrUserIDNotFound is returned when the user id is missing from context.
	ErrUserIDNotFound = errors.New("user-id not found")
)
