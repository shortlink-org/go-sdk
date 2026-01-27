// Package authjwt provides JWT validation for gRPC services.
//
// This package validates JWT tokens at service boundaries using JWKS for
// signature verification. It is designed to work with Oathkeeper's id_token
// mutator but can be used with any RS256 JWT issuer.
//
// Security considerations:
// - Always validate issuer and audience claims
// - Use short token TTL (15 min recommended)
// - JWKS should be fetched over HTTPS in production
// - Consider rate limiting on JWKS refresh
// - Clock skew tolerance is 30 seconds by default
package authjwt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrMissingToken indicates no token was provided.
var ErrMissingToken = errors.New("missing authorization token")

const (
	// DefaultLeeway is the default clock skew tolerance.
	DefaultLeeway = 30 * time.Second
)

// Validator validates JWT tokens.
type Validator struct {
	jwks          JWKSFetcher
	issuer        string
	audience      string
	skipAudience  bool
	skipIssuer    bool
	leeway        time.Duration
	customKeyfunc jwt.Keyfunc
}

// ValidatorConfig configures the JWT validator.
type ValidatorConfig struct {
	// JWKSURL is the URL to fetch JWKS from
	JWKSURL string
	// Issuer is the expected token issuer (iss claim)
	Issuer string
	// Audience is the expected audience (aud claim)
	Audience string
	// SkipAudience skips audience validation (not recommended)
	SkipAudience bool
	// SkipIssuer skips issuer validation (not recommended)
	SkipIssuer bool
	// Leeway is the clock skew tolerance (default: 30 seconds)
	Leeway time.Duration
	// JWKSCacheTTL is how long to cache JWKS (default: 1 hour)
	JWKSCacheTTL time.Duration
	// JWKSHTTPTimeout is the HTTP timeout for JWKS fetch (default: 10 seconds)
	JWKSHTTPTimeout time.Duration
	// JWKSBackoffMin is the minimum backoff after failed JWKS fetch (default: 500ms)
	JWKSBackoffMin time.Duration
	// JWKSBackoffMax is the maximum backoff after failed JWKS fetch (default: 30s)
	JWKSBackoffMax time.Duration
	// KeyFetcher overrides the default JWKS-based key lookup (for testing)
	KeyFetcher JWKSFetcher
	// CustomKeyfunc overrides the default JWKS-based key lookup (for testing)
	CustomKeyfunc jwt.Keyfunc
	// Clock overrides time source for JWKS (for testing)
	Clock Clock
}

// NewValidator creates a new JWT validator.
func NewValidator(cfg ValidatorConfig) (*Validator, error) {
	if cfg.JWKSURL == "" && cfg.CustomKeyfunc == nil && cfg.KeyFetcher == nil {
		return nil, ErrJWKSURLRequired
	}

	if !cfg.SkipIssuer && cfg.Issuer == "" {
		return nil, ErrIssuerRequired
	}

	if !cfg.SkipAudience && cfg.Audience == "" {
		return nil, ErrAudienceRequired
	}

	if cfg.Leeway == 0 {
		cfg.Leeway = DefaultLeeway
	}

	validator := &Validator{
		issuer:        cfg.Issuer,
		audience:      cfg.Audience,
		skipAudience:  cfg.SkipAudience,
		skipIssuer:    cfg.SkipIssuer,
		leeway:        cfg.Leeway,
		customKeyfunc: cfg.CustomKeyfunc,
	}

	if cfg.KeyFetcher != nil {
		validator.jwks = cfg.KeyFetcher
	} else if cfg.JWKSURL != "" {
		validator.jwks = NewJWKSFetcher(JWKSConfig{
			URL:         cfg.JWKSURL,
			CacheTTL:    cfg.JWKSCacheTTL,
			HTTPTimeout: cfg.JWKSHTTPTimeout,
			BackoffMin:  cfg.JWKSBackoffMin,
			BackoffMax:  cfg.JWKSBackoffMax,
			Clock:       cfg.Clock,
		})
	}

	return validator, nil
}

// ValidateResult contains the result of token validation.
type ValidateResult struct {
	Claims *Claims
	Valid  bool
	Error  error
}

// Validate validates a JWT token string.
func (v *Validator) Validate(ctx context.Context, tokenString string) ValidateResult {
	if tokenString == "" {
		return ValidateResult{Error: ErrMissingToken}
	}

	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return ValidateResult{Error: ErrMissingToken}
	}

	// Remove Bearer prefix if present (case-insensitive)
	if parts := strings.Fields(tokenString); len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		tokenString = parts[1]
	}

	// Build parser options
	opts := []jwt.ParserOption{
		jwt.WithLeeway(v.leeway),
		jwt.WithValidMethods([]string{"RS256"}),
	}

	if !v.skipIssuer && v.issuer != "" {
		opts = append(opts, jwt.WithIssuer(v.issuer))
	}

	if !v.skipAudience && v.audience != "" {
		opts = append(opts, jwt.WithAudience(v.audience))
	}

	opts = append(opts, jwt.WithExpirationRequired())

	parser := jwt.NewParser(opts...)

	// Determine keyfunc
	var keyfunc jwt.Keyfunc
	if v.customKeyfunc != nil {
		keyfunc = v.customKeyfunc
	} else {
		keyfunc = v.jwks.KeyFunc(ctx)
	}

	// Parse and validate
	claims := &Claims{}

	token, err := parser.ParseWithClaims(tokenString, claims, keyfunc)
	if err != nil {
		if isKnownValidationError(err) {
			return ValidateResult{Error: err}
		}

		return ValidateResult{Error: fmt.Errorf("%w: %v", ErrInvalidToken, err)}
	}

	if !token.Valid {
		return ValidateResult{Error: jwt.ErrTokenInvalidClaims}
	}

	return ValidateResult{
		Claims: claims,
		Valid:  true,
	}
}

// Close releases resources.
func (v *Validator) Close() error {
	if v.jwks != nil {
		return v.jwks.Close()
	}

	return nil
}
