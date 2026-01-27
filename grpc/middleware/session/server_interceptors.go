package sessioninterceptor

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shortlink-org/go-sdk/auth/session"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// expectedMethodParts is the expected number of parts when splitting a gRPC method name.
	expectedMethodParts = 3
)

// skipMethodPrefixes defines gRPC methods that should bypass session validation.
// Includes health checks and reflection services (both v1 and v1alpha).
var skipMethodPrefixes = []string{
	"/grpc.health.v1.Health/",
	"/grpc.reflection.v1.ServerReflection/",
	"/grpc.reflection.v1alpha.ServerReflection/",
}

var (
	tracerServer = otel.Tracer("session.interceptor.server")

	authIdentityResolutionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_identity_resolutions_total",
			Help: "Total number of user identity resolution attempts in gRPC interceptors.",
		},
		[]string{"source", "outcome", "reason"},
	)

	authIdentityResolutionSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "auth_identity_resolution_seconds",
			Help:    "Time spent resolving user identity in gRPC interceptors.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"source", "outcome", "reason"},
	)
)

// SessionUnaryServerInterceptor extracts user identity from incoming gRPC metadata.
// It looks for:
// 1) user-id in metadata (set by BFF).
// 2) authorization header for JWT validation (optional).
func SessionUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if shouldSkipMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		return handleUnarySession(ctx, req, info, handler)
	}
}

func handleUnarySession(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	svc, method := splitFullMethodName(info.FullMethod)

	ctx, span := tracerServer.Start(ctx, "ResolveUserIdentity")
	defer span.End()

	span.SetAttributes(
		attribute.String("rpc.system", "grpc"),
		attribute.String("rpc.service", svc),
		attribute.String("rpc.method", method),
	)

	start := time.Now()
	userID, source, err := resolveUserIdentity(ctx, span)

	outcome := "success"
	reason := "ok"

	if err != nil {
		code, reasonStr := classifyAuthError(err)
		reason = reasonStr
		outcome = "error"

		span.SetAttributes(
			attribute.Int("rpc.grpc.status_code", int(code)),
		)

		observeIdentityResolution(ctx, source, outcome, reason, start)

		return nil, status.Error(code, err.Error())
	}

	observeIdentityResolution(ctx, source, outcome, reason, start)

	span.SetAttributes(attribute.String("enduser.id", userID))
	span.SetStatus(otelcodes.Ok, "user identity resolved")

	ctx = session.WithUserID(ctx, userID)

	resp, err := handler(ctx, req)

	grpcCode := grpcCodes.OK
	if err != nil {
		grpcCode = status.Code(err)
	}

	span.SetAttributes(
		attribute.Int("rpc.grpc.status_code", int(grpcCode)),
	)

	return resp, err
}

// SessionStreamServerInterceptor applies identity resolution for streaming RPCs.
func SessionStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if shouldSkipMethod(info.FullMethod) {
			return handler(srv, stream)
		}

		return handleStreamSession(srv, stream, info, handler)
	}
}

func handleStreamSession(
	srv any,
	stream grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	base := stream.Context()
	svc, method := splitFullMethodName(info.FullMethod)

	ctx, span := tracerServer.Start(base, "ResolveUserIdentityStream")
	defer span.End()

	span.SetAttributes(
		attribute.String("rpc.system", "grpc"),
		attribute.String("rpc.service", svc),
		attribute.String("rpc.method", method),
	)

	start := time.Now()
	userID, source, err := resolveUserIdentity(ctx, span)
	outcome, reason := "success", "ok"

	if err != nil {
		code, reasonStr := classifyAuthError(err)
		span.SetAttributes(attribute.Int("rpc.grpc.status_code", int(code)))
		observeIdentityResolution(ctx, source, "error", reasonStr, start)

		return status.Error(code, err.Error())
	}

	observeIdentityResolution(ctx, source, outcome, reason, start)
	span.SetAttributes(attribute.String("enduser.id", userID))
	span.SetStatus(otelcodes.Ok, "user identity resolved")

	ctx = session.WithUserID(ctx, userID)
	wrapped := &sessionWrappedServerStream{ServerStream: stream, wrappedCtx: ctx}
	err = handler(srv, wrapped)

	grpcCode := grpcCodes.OK
	if err != nil {
		grpcCode = status.Code(err)
	}

	span.SetAttributes(attribute.Int("rpc.grpc.status_code", int(grpcCode)))

	return err
}

// resolveUserIdentity extracts user identity from gRPC metadata.
// Priority: metadata user-id, then context fallback.
func resolveUserIdentity(ctx context.Context, span trace.Span) (string, string, error) {
	// 1. Try metadata first (user-id set by BFF)
	if userID, ok := resolveFromMetadata(ctx, span); ok {
		return userID, "metadata", nil
	}

	// 2. Fallback to context (if already set by upstream)
	userID, err := resolveFromContext(ctx, span)
	if err != nil {
		return "", "context", err
	}

	return userID, "context", nil
}

// resolveFromMetadata reads user-id from gRPC metadata.
func resolveFromMetadata(ctx context.Context, span trace.Span) (string, bool) {
	incomingMD, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}

	// Try user-id key first
	values := incomingMD.Get(userIDKey)
	if len(values) > 0 {
		userID := strings.TrimSpace(values[0])
		if userID != "" {
			span.SetAttributes(
				attribute.String("auth.user_id.source", "metadata"),
				attribute.String("auth.user_id", userID),
			)

			return userID, true
		}
	}

	return "", false
}

func resolveFromContext(ctx context.Context, span trace.Span) (string, error) {
	userID, err := session.GetUserID(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())

		return "", ErrServerMissingUserID
	}

	span.SetAttributes(
		attribute.String("auth.user_id.source", "context"),
		attribute.String("auth.user_id", userID),
	)

	return userID, nil
}

// --- Stream Wrapper ---

// sessionWrappedServerStream wraps a gRPC server stream with a custom context.
//
//nolint:containedctx // Required for grpc stream context override pattern
type sessionWrappedServerStream struct {
	grpc.ServerStream

	wrappedCtx context.Context
}

func (s *sessionWrappedServerStream) Context() context.Context {
	return s.wrappedCtx
}

// --- Error Mapping ---

func classifyAuthError(err error) (grpcCodes.Code, string) {
	switch {
	case errors.Is(err, ErrServerMissingMetadata):
		return grpcCodes.Unauthenticated, "missing_metadata"

	case errors.Is(err, ErrServerMissingUserID):
		return grpcCodes.Unauthenticated, "missing_user_id"

	default:
		return grpcCodes.Internal, "internal_error"
	}
}

// --- Metrics (with exemplars) ---

func observeIdentityResolution(ctx context.Context, source, outcome, reason string, start time.Time) {
	duration := time.Since(start).Seconds()

	authIdentityResolutionTotal.
		WithLabelValues(source, outcome, reason).
		Inc()

	obs := authIdentityResolutionSeconds.
		WithLabelValues(source, outcome, reason)

	if eo, ok := obs.(prometheus.ExemplarObserver); ok {
		if sc := trace.SpanContextFromContext(ctx); sc.IsSampled() && sc.HasTraceID() {
			eo.ObserveWithExemplar(duration, prometheus.Labels{
				"trace_id": sc.TraceID().String(),
			})

			return
		}
	}

	obs.Observe(duration)
}

// --- Utils ---

func shouldSkipMethod(fullMethod string) bool {
	for _, prefix := range skipMethodPrefixes {
		if strings.HasPrefix(fullMethod, prefix) {
			return true
		}
	}

	return false
}

func splitFullMethodName(full string) (string, string) {
	parts := strings.Split(full, "/")
	if len(parts) != expectedMethodParts {
		return "", ""
	}

	return parts[1], parts[2]
}
