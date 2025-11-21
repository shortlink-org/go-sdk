//nolint:revive // package name uses underscore for consistency with project structure
package session_interceptor

import (
	"errors"
	"fmt"
)

// ErrEmptyUserID is returned when user-id is empty.
var ErrEmptyUserID = errors.New("attachUserMetadata: user-id is empty")

// ErrServerMissingMetadata is returned when incoming metadata is missing.
var ErrServerMissingMetadata = errors.New("session_interceptor: incoming metadata missing")

// ErrServerMissingUserID is returned when user-id is missing in incoming metadata.
var ErrServerMissingUserID = errors.New("session_interceptor: user-id missing in incoming metadata")

// UserIDMismatchError indicates that metadata user-id doesn't match session user-id.
type UserIDMismatchError struct {
	MetadataID string
	SessionID  string
}

func (e UserIDMismatchError) Error() string {
	return fmt.Sprintf("user-id mismatch: metadata=%s session=%s", e.MetadataID, e.SessionID)
}

// SessionLoadError indicates that session loading failed.
type SessionLoadError struct {
	Err error
}

func (e *SessionLoadError) Error() string {
	return "failed to load session: " + e.Err.Error()
}

func (e *SessionLoadError) Unwrap() error {
	return e.Err
}

// UserIDNotFoundError indicates that user-id cannot be retrieved from context.
type UserIDNotFoundError struct {
	Err error
}

func (e *UserIDNotFoundError) Error() string {
	return "user-id not found in context: " + e.Err.Error()
}

func (e *UserIDNotFoundError) Unwrap() error {
	return e.Err
}
