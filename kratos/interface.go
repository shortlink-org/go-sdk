package kratos

import (
	"context"
)

// KratosClient defines the interface for Kratos Admin API client operations.
// This interface allows for easy mocking in tests.
type KratosClient interface {
	// GetUserEmail retrieves user email by user ID from Kratos Admin API.
	// Returns email and error. If user not found or email is missing, returns error.
	GetUserEmail(ctx context.Context, userID string) (string, error)
}

// Ensure Client implements KratosClient interface
var _ KratosClient = (*Client)(nil)

