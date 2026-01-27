package authforward

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestWithToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{"valid token", "Bearer abc123", "Bearer abc123"},
		{"empty token", "", ""},
		{"raw token", "abc123", "abc123"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := WithToken(context.Background(), testCase.token)
			got := TokenFromContext(ctx)
			assert.Equal(t, testCase.expected, got)
		})
	}
}

func TestTokenFromIncomingMetadata_ValidHeader(t *testing.T) {
	t.Parallel()

	md := metadata.Pairs("authorization", "Bearer token123")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	got := TokenFromIncomingMetadata(ctx)
	assert.Equal(t, "Bearer token123", got)
}

func TestTokenFromIncomingMetadata_NoMetadata(t *testing.T) {
	t.Parallel()

	got := TokenFromIncomingMetadata(context.Background())
	assert.Empty(t, got)
}

func TestTokenFromIncomingMetadata_EmptyAuth(t *testing.T) {
	t.Parallel()

	md := metadata.Pairs("authorization", "")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	got := TokenFromIncomingMetadata(ctx)
	assert.Empty(t, got)
}

func TestTokenFromIncomingMetadata_MultipleValues(t *testing.T) {
	t.Parallel()

	md := metadata.Pairs(
		"authorization", "Bearer first",
		"authorization", "Bearer second",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	got := TokenFromIncomingMetadata(ctx)
	assert.Equal(t, "Bearer first", got)
}

func TestTokenFromIncomingMetadata_WhitespaceTrimmed(t *testing.T) {
	t.Parallel()

	md := metadata.Pairs("authorization", "  Bearer token  ")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	got := TokenFromIncomingMetadata(ctx)
	assert.Equal(t, "Bearer token", got)
}

func TestSetOutgoingToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		existingToken string
		newToken      string
		expected      string
	}{
		{
			name:          "set new token",
			existingToken: "",
			newToken:      "Bearer new",
			expected:      "Bearer new",
		},
		{
			name:          "replace existing token",
			existingToken: "Bearer old",
			newToken:      "Bearer new",
			expected:      "Bearer new",
		},
		{
			name:          "empty token does nothing",
			existingToken: "Bearer old",
			newToken:      "",
			expected:      "Bearer old",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// Set existing token if any
			if testCase.existingToken != "" {
				md := metadata.Pairs("authorization", testCase.existingToken)
				ctx = metadata.NewOutgoingContext(ctx, md)
			}

			// Set new token
			ctx = SetOutgoingToken(ctx, testCase.newToken)

			// Verify
			got := TokenFromOutgoingMetadata(ctx)
			assert.Equal(t, testCase.expected, got)
		})
	}
}

func TestSetOutgoingToken_NoAccumulation(t *testing.T) {
	t.Parallel()

	// This test verifies that multiple calls to SetOutgoingToken
	// do not accumulate multiple authorization values
	ctx := context.Background()

	// Set token multiple times
	ctx = SetOutgoingToken(ctx, "Bearer first")
	ctx = SetOutgoingToken(ctx, "Bearer second")
	ctx = SetOutgoingToken(ctx, "Bearer third")

	// Get metadata and verify only one authorization value
	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)

	values := md.Get("authorization")
	require.Len(t, values, 1, "should have exactly one authorization value")
	assert.Equal(t, "Bearer third", values[0])
}

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"valid bearer", "Bearer abc123", "abc123"},
		{"no prefix", "abc123", ""},
		{"empty", "", ""},
		{"wrong prefix", "Basic abc123", ""},
		{"lowercase bearer", "bearer abc123", ""},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := ExtractBearerToken(testCase.input)
			assert.Equal(t, testCase.expected, got)
		})
	}
}

func TestFormatBearerToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"raw token", "abc123", "Bearer abc123"},
		{"already formatted", "Bearer abc123", "Bearer abc123"},
		{"empty", "", ""},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := FormatBearerToken(testCase.input)
			assert.Equal(t, testCase.expected, got)
		})
	}
}

func TestCaptureAndForward(t *testing.T) {
	t.Parallel()

	// Simulate server receiving request with authorization
	incomingMD := metadata.Pairs("authorization", "Bearer user-token")
	serverCtx := metadata.NewIncomingContext(context.Background(), incomingMD)

	// Server interceptor captures token
	serverCtx = captureToken(serverCtx)

	// Verify token is in context
	token := TokenFromContext(serverCtx)
	require.Equal(t, "Bearer user-token", token)

	// Client interceptor forwards token
	clientCtx := forwardToken(serverCtx, "/test.Service/Method")

	// Verify token is in outgoing metadata
	outToken := TokenFromOutgoingMetadata(clientCtx)
	assert.Equal(t, "Bearer user-token", outToken)
}

func TestForwardToken_DoesNotDuplicate(t *testing.T) {
	t.Parallel()

	// Token already in outgoing metadata
	md := metadata.Pairs("authorization", "Bearer existing")
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	// Also set in context
	ctx = WithToken(ctx, "Bearer from-context")

	// Forward should not overwrite existing
	ctx = forwardToken(ctx, "/test.Service/Method")

	outMD, _ := metadata.FromOutgoingContext(ctx)
	values := outMD.Get("authorization")

	require.Len(t, values, 1)
	assert.Equal(t, "Bearer existing", values[0])
}
