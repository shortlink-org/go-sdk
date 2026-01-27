package authjwt

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Sentinel errors for JWKS operations.
// JWT validation errors should be checked directly via github.com/golang-jwt/jwt/v5.
var (
	// ErrJWKSURLRequired is returned when neither JWKSURL nor CustomKeyfunc is provided.
	ErrJWKSURLRequired = errors.New("JWKSURL or CustomKeyfunc is required")
	// ErrKeyNotFound is returned when the requested kid is not in JWKS.
	ErrKeyNotFound = errors.New("key not found in JWKS")
	// ErrNoValidKeys is returned when JWKS contains no valid RSA keys.
	ErrNoValidKeys = errors.New("no valid RSA keys in JWKS")
	// ErrUnexpectedStatus is returned when JWKS endpoint returns non-200 status.
	ErrUnexpectedStatus = errors.New("unexpected JWKS response status")
	// ErrMissingKid is returned when token header doesn't contain kid.
	ErrMissingKid = errors.New("missing kid in token header")
	// ErrUnexpectedSignMethod is returned when token uses unexpected signing method.
	ErrUnexpectedSignMethod = errors.New("unexpected signing method")
)

// errorMappings defines how errors map to gRPC status codes.
//
//nolint:gochecknoglobals // Intentional mapping table
var errorMappings = []struct {
	err     error
	code    codes.Code
	message string
}{
	{jwt.ErrTokenExpired, codes.Unauthenticated, "token expired"},
	{jwt.ErrTokenNotValidYet, codes.Unauthenticated, "token not yet valid"},
	{jwt.ErrTokenMalformed, codes.InvalidArgument, "malformed token"},
	{jwt.ErrTokenSignatureInvalid, codes.Unauthenticated, "invalid token signature"},
	{jwt.ErrTokenInvalidAudience, codes.PermissionDenied, "invalid audience"},
	{jwt.ErrTokenInvalidIssuer, codes.PermissionDenied, "invalid issuer"},
	{ErrKeyNotFound, codes.Unauthenticated, "unknown signing key"},
	{ErrMissingKid, codes.InvalidArgument, "missing key id in token"},
	{ErrUnexpectedSignMethod, codes.InvalidArgument, "unsupported signing method"},
	{ErrNoValidKeys, codes.Internal, "authentication service unavailable"},
	{ErrUnexpectedStatus, codes.Internal, "authentication service unavailable"},
}

// ToGRPCStatus converts a JWT validation error to an appropriate gRPC status.
// Use at the gRPC boundary (interceptors) to return proper status codes.
func ToGRPCStatus(err error) error {
	if err == nil {
		return nil
	}

	for _, mapping := range errorMappings {
		if errors.Is(err, mapping.err) {
			return status.Error(mapping.code, mapping.message)
		}
	}

	return status.Error(codes.Unauthenticated, "authentication failed")
}
