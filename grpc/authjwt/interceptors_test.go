package authjwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/shortlink-org/go-sdk/auth/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	interceptorKeyOnce sync.Once
	interceptorPrivKey *rsa.PrivateKey
	interceptorPubKey  *rsa.PublicKey
)

func getInterceptorKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()

	interceptorKeyOnce.Do(func() {
		var err error
		interceptorPrivKey, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		interceptorPubKey = &interceptorPrivKey.PublicKey
	})

	return interceptorPrivKey, interceptorPubKey
}

func createInterceptorToken(t *testing.T, claims Claims) string {
	t.Helper()

	priv, _ := getInterceptorKeys(t)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-kid"

	tokenString, err := token.SignedString(priv)
	require.NoError(t, err)

	return tokenString
}

func TestValidateRequest_MissingToken(t *testing.T) {
	t.Parallel()

	_, pub := getInterceptorKeys(t)
	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		Audience:      "shortlink-api",
		CustomKeyfunc: func(_ *jwt.Token) (any, error) { return pub, nil },
	})
	require.NoError(t, err)

	_, gotErr := validateRequest(context.Background(), validator, "/test.Service/Method", nil)
	require.Error(t, gotErr)
	assert.Equal(t, codes.Unauthenticated, status.Code(gotErr))
}

func TestValidateRequest_MultipleAuthHeaders(t *testing.T) {
	t.Parallel()

	_, pub := getInterceptorKeys(t)
	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		Audience:      "shortlink-api",
		CustomKeyfunc: func(_ *jwt.Token) (any, error) { return pub, nil },
	})
	require.NoError(t, err)

	md := metadata.Pairs(
		"authorization", "Bearer first",
		"authorization", "Bearer second",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, gotErr := validateRequest(ctx, validator, "/test.Service/Method", nil)
	require.Error(t, gotErr)
	assert.Equal(t, codes.InvalidArgument, status.Code(gotErr))
}

func TestValidateRequest_SetsSessionClaims(t *testing.T) {
	t.Parallel()

	_, pub := getInterceptorKeys(t)
	validator, err := NewValidator(ValidatorConfig{
		Issuer:        "https://shortlink.best",
		Audience:      "shortlink-api",
		CustomKeyfunc: func(_ *jwt.Token) (any, error) { return pub, nil },
	})
	require.NoError(t, err)

	token := createInterceptorToken(t, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "https://shortlink.best",
			Audience:  jwt.ClaimStrings{"shortlink-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Email: "test@example.com",
	})

	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	newCtx, gotErr := validateRequest(ctx, validator, "/test.Service/Method", nil)
	require.NoError(t, gotErr)

	claims := ClaimsFromContext(newCtx)
	require.NotNil(t, claims)
	assert.Equal(t, "user-123", claims.Subject)

	userID, err := session.GetUserID(newCtx)
	require.NoError(t, err)
	assert.Equal(t, "user-123", userID)
}
