package sessioninterceptor

import "errors"

// ErrServerMissingMetadata is returned when incoming metadata is missing.
var ErrServerMissingMetadata = errors.New("session_interceptor: incoming metadata missing")

// ErrServerMissingUserID is returned when user-id is missing in incoming metadata.
var ErrServerMissingUserID = errors.New("session_interceptor: user-id missing in incoming metadata")
