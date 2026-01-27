package authjwt

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shortlink-org/go-sdk/auth/session"
	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	reflectionMethod  = "/grpc.reflection"
	healthCheckMethod = "/grpc.health"
	authorizationKey  = "authorization"
)

var (
	tracer = otel.Tracer("authjwt")

	jwtValidationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_jwt_validations_total",
			Help: "Total JWT validation attempts in gRPC interceptors",
		},
		[]string{"outcome", "method"},
	)

	jwtValidationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_jwt_validation_seconds",
			Help:    "Time spent validating JWT in gRPC interceptors",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"outcome"},
	)
)

// InterceptorConfig configures the JWT interceptor behavior.
type InterceptorConfig struct {
	// SkipMethods is a list of method prefixes to skip authentication
	// Default: /grpc.reflection, /grpc.health
	SkipMethods []string
	// Logger logs authentication failures (optional).
	Logger logger.Logger
}

// UnaryServerInterceptor validates JWT tokens on incoming unary requests.
// It extracts the token from the "authorization" metadata key, validates it,
// and stores the claims in context.
func UnaryServerInterceptor(validator *Validator, cfg InterceptorConfig) grpc.UnaryServerInterceptor {
	skipMethods := mergeSkipMethods(cfg.SkipMethods)

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Skip configured methods
		if shouldSkip(info.FullMethod, skipMethods) {
			return handler(ctx, req)
		}

		ctx, err := validateRequest(ctx, validator, info.FullMethod, cfg.Logger)
		if err != nil {
			return nil, err
		}

		return handler(ctx, req)
	}
}

// StreamServerInterceptor validates JWT tokens on incoming stream requests.
func StreamServerInterceptor(validator *Validator, cfg InterceptorConfig) grpc.StreamServerInterceptor {
	skipMethods := mergeSkipMethods(cfg.SkipMethods)

	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Skip configured methods
		if shouldSkip(info.FullMethod, skipMethods) {
			return handler(srv, stream)
		}

		ctx, err := validateRequest(stream.Context(), validator, info.FullMethod, cfg.Logger)
		if err != nil {
			return err
		}

		return handler(srv, &wrappedServerStream{ServerStream: stream, wrappedCtx: ctx})
	}
}

func validateRequest(ctx context.Context, validator *Validator, method string, log logger.Logger) (context.Context, error) {
	start := time.Now()

	ctx, span := tracer.Start(ctx, "authjwt.ValidateToken",
		trace.WithAttributes(
			attribute.String("rpc.method", method),
		),
	)
	defer span.End()

	// Extract token from metadata
	token, tokenErr := extractToken(ctx)
	if tokenErr != nil {
		outcome := classifyError(tokenErr)
		recordMetrics(outcome, method, start)
		span.RecordError(tokenErr)
		span.SetStatus(codes.Error, tokenErr.Error())

		if log != nil {
			log.Warn("jwt validation failed",
				slog.String("method", method),
				slog.String("reason", outcome),
			)
		}

		return nil, ToGRPCStatus(tokenErr)
	}

	// Validate token
	result := validator.Validate(ctx, token)
	if result.Error != nil {
		outcome := classifyError(result.Error)
		recordMetrics(outcome, method, start)
		span.RecordError(result.Error)
		span.SetStatus(codes.Error, result.Error.Error())

		if log != nil {
			log.Warn("jwt validation failed",
				slog.String("method", method),
				slog.String("reason", outcome),
			)
		}

		return nil, ToGRPCStatus(result.Error)
	}

	// Success
	recordMetrics("success", method, start)

	span.SetAttributes(
		attribute.String("enduser.id", result.Claims.Subject),
		attribute.String("enduser.email", result.Claims.Email),
	)
	span.SetStatus(codes.Ok, "token validated")

	// Store claims in context
	ctx = WithClaims(ctx, result.Claims)
	ctx = session.WithUserID(ctx, result.Claims.Subject)
	ctx = session.WithClaims(ctx, toSessionClaims(result.Claims))

	return ctx, nil
}

func toSessionClaims(claims *Claims) *session.Claims {
	if claims == nil {
		return nil
	}

	out := &session.Claims{
		Subject:    claims.Subject,
		Email:      claims.Email,
		Name:       claims.Name,
		IdentityID: claims.IdentityID,
		SessionID:  claims.SessionID,
		Metadata:   claims.Metadata,
		Issuer:     claims.Issuer,
	}

	if claims.IssuedAt != nil {
		out.IssuedAt = claims.IssuedAt.Unix()
	}

	if claims.ExpiresAt != nil {
		out.ExpiresAt = claims.ExpiresAt.Unix()
	}

	return out
}

func classifyError(err error) string {
	switch {
	case errors.Is(err, ErrMissingToken):
		return "missing_token"
	case errors.Is(err, ErrMultipleAuthHeaders):
		return "multiple_auth_headers"
	case errors.Is(err, jwt.ErrTokenExpired):
		return "expired"
	case errors.Is(err, jwt.ErrTokenMalformed):
		return "malformed"
	case errors.Is(err, jwt.ErrTokenSignatureInvalid):
		return "invalid_signature"
	case errors.Is(err, jwt.ErrTokenInvalidAudience):
		return "invalid_audience"
	case errors.Is(err, jwt.ErrTokenInvalidIssuer):
		return "invalid_issuer"
	case errors.Is(err, ErrKeyNotFound):
		return "unknown_kid"
	case errors.Is(err, ErrNoValidKeys), errors.Is(err, ErrUnexpectedStatus), errors.Is(err, ErrJWKSBackoff):
		return "jwks_unavailable"
	default:
		return "invalid_token"
	}
}

func extractToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ErrMissingToken
	}

	values := md.Get(authorizationKey)
	if len(values) == 0 {
		return "", ErrMissingToken
	}

	if len(values) > 1 {
		return "", ErrMultipleAuthHeaders
	}

	token := strings.TrimSpace(values[0])
	if token == "" {
		return "", ErrMissingToken
	}

	return token, nil
}

func shouldSkip(method string, skipMethods []string) bool {
	for _, prefix := range skipMethods {
		if strings.HasPrefix(method, prefix) {
			return true
		}
	}

	return false
}

func mergeSkipMethods(custom []string) []string {
	defaults := []string{reflectionMethod, healthCheckMethod}

	return append(defaults, custom...)
}

func recordMetrics(outcome, method string, start time.Time) {
	jwtValidationTotal.WithLabelValues(outcome, method).Inc()
	jwtValidationSeconds.WithLabelValues(outcome).Observe(time.Since(start).Seconds())
}

// Stream wrapper.
//
//nolint:containedctx // Required for grpc stream context override pattern
type wrappedServerStream struct {
	grpc.ServerStream

	wrappedCtx context.Context
}

func (wrapper *wrappedServerStream) Context() context.Context {
	return wrapper.wrappedCtx
}
