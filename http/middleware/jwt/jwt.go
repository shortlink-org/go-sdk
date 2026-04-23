package jwt_middleware

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/auth/session"
	"github.com/shortlink-org/go-sdk/config"
	session_interceptor "github.com/shortlink-org/go-sdk/grpc/middleware/session"
)

const (
	tracerName = "github.com/shortlink-org/go-sdk/http/middleware/jwt"

	authSchemeParts = 2 // "Bearer" + token from Authorization header
)

// jwtMiddleware holds the middleware configuration.
type jwtMiddleware struct {
	tracer     trace.Tracer
	cfg        *config.Config
	parser     *jwt.Parser
	propagator propagation.TextMapPropagator
}

// JWT creates a new JWT authentication middleware.
// This middleware extracts and validates JWT tokens from the Authorization header.
// The JWT is expected to be issued by Oathkeeper's id_token mutator.
//
// Configuration:
//   - AUTH_LOGIN_URL: URL to redirect unauthenticated users (default: /auth/login)
//
// Note: Signature verification is skipped because we trust Oathkeeper.
// The token is validated by Oathkeeper before reaching the BFF.
//
// Trace propagation: This middleware extracts trace context from incoming headers
// (traceparent, b3, uber-trace-id) to maintain distributed tracing across services.
func JWT(cfg *config.Config) func(next http.Handler) http.Handler {
	cfg.SetDefault("AUTH_LOGIN_URL", "/auth/login")

	// Use composite propagator for W3C TraceContext and Baggage
	prop := otel.GetTextMapPropagator()

	return jwtMiddleware{
		tracer: otel.Tracer(tracerName),
		cfg:    cfg,
		parser: jwt.NewParser(
			jwt.WithoutClaimsValidation(), // Skip expiration validation (Oathkeeper handles it)
		),
		propagator: prop,
	}.middleware
}

// oathkeeperClaims represents the JWT claims from Oathkeeper id_token mutator.
//
//nolint:tagliatelle // JSON field names match Oathkeeper id_token mutator schema.
type oathkeeperClaims struct {
	jwt.RegisteredClaims

	Email      string         `json:"email"`
	Name       string         `json:"name"`
	IdentityID string         `json:"identity_id"`
	SessionID  string         `json:"session_id"`
	Metadata   map[string]any `json:"metadata"`
}

func (j jwtMiddleware) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, req *http.Request) {
		// Extract trace context from incoming headers (traceparent, b3, uber-trace-id)
		// This ensures trace continuity from Oathkeeper -> BFF -> downstream services
		ctx := j.propagator.Extract(req.Context(), propagation.HeaderCarrier(req.Header))

		ctx, span := j.tracer.Start(ctx, "jwt.validate",
			trace.WithAttributes(attribute.String("component", "jwt_middleware")),
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		// Extract token from Authorization header
		tokenString := extractBearerToken(req)
		if tokenString == "" {
			span.SetStatus(codes.Error, "missing authorization header")
			j.handleUnauthorized(responseWriter, req)

			return
		}

		// Parse JWT without signature verification (we trust Oathkeeper)
		token, _, err := j.parser.ParseUnverified(tokenString, &oathkeeperClaims{})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			j.handleUnauthorized(responseWriter, req)

			return
		}

		oathClaims, ok := token.Claims.(*oathkeeperClaims)
		if !ok {
			span.SetStatus(codes.Error, "invalid claims type")
			j.handleUnauthorized(responseWriter, req)

			return
		}

		// Validate subject is present
		if oathClaims.Subject == "" {
			span.SetStatus(codes.Error, "missing subject in token")
			j.handleUnauthorized(responseWriter, req)

			return
		}

		// Convert to session.Claims
		claims := &session.Claims{
			Subject:    oathClaims.Subject,
			Email:      oathClaims.Email,
			Name:       oathClaims.Name,
			IdentityID: oathClaims.IdentityID,
			SessionID:  oathClaims.SessionID,
			Metadata:   oathClaims.Metadata,
			Issuer:     oathClaims.Issuer,
		}

		if oathClaims.IssuedAt != nil {
			claims.IssuedAt = oathClaims.IssuedAt.Unix()
		}

		if oathClaims.ExpiresAt != nil {
			claims.ExpiresAt = oathClaims.ExpiresAt.Unix()
		}

		span.SetStatus(codes.Ok, "token validated")
		span.SetAttributes(
			attribute.String("user.id", claims.Subject),
			attribute.String("user.email", claims.Email),
			attribute.String("session.id", claims.SessionID),
		)

		// Enrich context with claims and user ID
		ctx = session.WithClaims(ctx, claims)
		ctx = session.WithUserID(ctx, claims.Subject)

		// Store original Authorization header for gRPC propagation
		authHeader := req.Header.Get("Authorization")
		ctx = session_interceptor.WithAuthorization(ctx, authHeader)

		next.ServeHTTP(responseWriter, req.WithContext(ctx))
	})
}

// extractBearerToken extracts the JWT from the Authorization header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	parts := strings.SplitN(auth, " ", authSchemeParts)
	if len(parts) != authSchemeParts || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}

	return parts[1]
}

// handleUnauthorized handles unauthorized requests.
// For API requests (Accept: application/json), returns 401.
// For browser requests, redirects to login page.
func (j jwtMiddleware) handleUnauthorized(responseWriter http.ResponseWriter, req *http.Request) {
	accept := req.Header.Get("Accept")

	// API request - return JSON error
	if strings.Contains(accept, "application/json") {
		responseWriter.Header().Set("Content-Type", "application/json")
		responseWriter.WriteHeader(http.StatusUnauthorized)

		_, writeErr := responseWriter.Write([]byte(`{"error":"unauthorized","message":"authentication required"}`))
		if writeErr != nil {
			return
		}

		return
	}

	// Browser request - redirect to login
	http.Redirect(responseWriter, req, j.cfg.GetString("AUTH_LOGIN_URL"), http.StatusFound)
}
