package authjwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test key for signing tokens.
var (
	testPrivateKey *rsa.PrivateKey
	testPublicKey  *rsa.PublicKey
)

func TestMain(m *testing.M) {
	var err error

	testPrivateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}

	testPublicKey = &testPrivateKey.PublicKey

	m.Run()
}

// mockKeyfunc returns the test public key for any token.
func mockKeyfunc(_ *jwt.Token) (any, error) {
	return testPublicKey, nil
}

func createTestToken(t *testing.T, claims Claims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key-1"

	tokenString, err := token.SignedString(testPrivateKey)
	require.NoError(t, err)

	return tokenString
}

func TestValidator_ValidToken(t *testing.T) {
	t.Parallel()

	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		Audience:      "shortlink-api",
		CustomKeyfunc: mockKeyfunc,
	})
	require.NoError(t, err)

	token := createTestToken(t, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "https://shortlink.best",
			Audience:  jwt.ClaimStrings{"shortlink-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Email: "test@example.com",
	})

	result := validator.Validate(context.Background(), token)
	require.True(t, result.Valid)
	require.NoError(t, result.Error)
	require.NotNil(t, result.Claims)
	assert.Equal(t, "user-123", result.Claims.Subject)
}

func TestValidator_EmptyToken(t *testing.T) {
	t.Parallel()

	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		SkipAudience:  true,
		CustomKeyfunc: mockKeyfunc,
	})
	require.NoError(t, err)

	result := validator.Validate(context.Background(), "")
	assert.False(t, result.Valid)
	assert.ErrorIs(t, result.Error, ErrMissingToken)
}

func TestValidator_InvalidTokenFormat(t *testing.T) {
	t.Parallel()

	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		SkipAudience:  true,
		CustomKeyfunc: mockKeyfunc,
	})
	require.NoError(t, err)

	result := validator.Validate(context.Background(), "not.a.valid.token")
	assert.False(t, result.Valid)
	assert.ErrorIs(t, result.Error, jwt.ErrTokenMalformed)
}

func TestValidator_ExpiredToken(t *testing.T) {
	t.Parallel()

	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		Audience:      "shortlink-api",
		CustomKeyfunc: mockKeyfunc,
	})
	require.NoError(t, err)

	token := createTestToken(t, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "https://shortlink.best",
			Audience:  jwt.ClaimStrings{"shortlink-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	})

	result := validator.Validate(context.Background(), token)
	assert.False(t, result.Valid)
	assert.ErrorIs(t, result.Error, jwt.ErrTokenExpired)
}

func TestValidator_WrongIssuer(t *testing.T) {
	t.Parallel()

	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		SkipAudience:  true,
		CustomKeyfunc: mockKeyfunc,
	})
	require.NoError(t, err)

	token := createTestToken(t, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "https://wrong-issuer.com",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	result := validator.Validate(context.Background(), token)
	assert.False(t, result.Valid)
}

func TestValidator_WrongAudience(t *testing.T) {
	t.Parallel()

	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		Audience:      "shortlink-api",
		CustomKeyfunc: mockKeyfunc,
	})
	require.NoError(t, err)

	token := createTestToken(t, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "https://shortlink.best",
			Audience:  jwt.ClaimStrings{"wrong-audience"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	result := validator.Validate(context.Background(), token)
	assert.False(t, result.Valid)
}

func TestValidator_BearerPrefix(t *testing.T) {
	t.Parallel()

	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		Audience:      "shortlink-api",
		CustomKeyfunc: mockKeyfunc,
	})
	require.NoError(t, err)

	token := "Bearer " + createTestToken(t, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-456",
			Issuer:    "https://shortlink.best",
			Audience:  jwt.ClaimStrings{"shortlink-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	result := validator.Validate(context.Background(), token)
	assert.True(t, result.Valid)
}

func TestValidator_SkipAudience(t *testing.T) {
	t.Parallel()

	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		SkipAudience:  true,
		CustomKeyfunc: mockKeyfunc,
	})
	require.NoError(t, err)

	// Token without audience should be valid
	token := createTestToken(t, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "https://shortlink.best",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	result := validator.Validate(context.Background(), token)
	assert.True(t, result.Valid)
}

func TestClaims_Context(t *testing.T) {
	t.Parallel()

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-789",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Email:      "test@example.com",
		Name:       "Test User",
		IdentityID: "identity-123",
	}

	ctx := WithClaims(context.Background(), claims)

	// Test retrieval
	got := ClaimsFromContext(ctx)
	require.NotNil(t, got)
	assert.Equal(t, "user-789", got.Subject)
	assert.Equal(t, "test@example.com", got.Email)

	// Test helpers
	assert.Equal(t, "user-789", GetSubject(ctx))
	assert.Equal(t, "test@example.com", GetEmail(ctx))
	assert.True(t, IsAuthenticated(ctx))

	// Test empty context
	assert.Nil(t, ClaimsFromContext(context.Background()))
	assert.Empty(t, GetSubject(context.Background()))
	assert.False(t, IsAuthenticated(context.Background()))
}

func TestValidator_NoIssuer(t *testing.T) {
	t.Parallel()

	// Validator without issuer check
	validator, err := NewValidator(ValidatorConfig{
		SkipAudience:  true,
		SkipIssuer:    true,
		CustomKeyfunc: mockKeyfunc,
	})
	require.NoError(t, err)

	token := createTestToken(t, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "any-issuer",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	result := validator.Validate(context.Background(), token)
	assert.True(t, result.Valid)
}

func TestNewValidator_RequiresIssuer(t *testing.T) {
	t.Parallel()

	_, err := NewValidator(ValidatorConfig{
		Audience:      "shortlink-api",
		SkipAudience:  true,
		CustomKeyfunc: mockKeyfunc,
	})
	assert.ErrorIs(t, err, ErrIssuerRequired)
}

func TestNewValidator_RequiresAudience(t *testing.T) {
	t.Parallel()

	_, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		CustomKeyfunc: mockKeyfunc,
	})
	assert.ErrorIs(t, err, ErrAudienceRequired)
}

func TestShouldSkip(t *testing.T) {
	t.Parallel()

	skipMethods := []string{"/grpc.reflection", "/grpc.health", "/custom.Skip"}

	tests := []struct {
		method   string
		expected bool
	}{
		{"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo", true},
		{"/grpc.health.v1.Health/Check", true},
		{"/custom.Skip/Method", true},
		{"/myservice.Service/Method", false},
		{"/other.Service/Other", false},
	}

	for _, testCase := range tests {
		t.Run(testCase.method, func(t *testing.T) {
			t.Parallel()

			got := shouldSkip(testCase.method, skipMethods)
			assert.Equal(t, testCase.expected, got)
		})
	}
}
