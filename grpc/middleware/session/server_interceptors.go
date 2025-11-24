package session_interceptor

import (
	"context"
	"errors"
	"strings"
	"time"

	ory "github.com/ory/client-go"
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

const reflectionMethod = "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo"

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

// SessionUnaryServerInterceptor ensures that every unary RPC request
// contains a valid authenticated user identity resolved from:
// 1) gRPC metadata
// 2) Ory session
// 3) context fallback
func SessionUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if info.FullMethod == reflectionMethod {
			return handler(ctx, req)
		}

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
			code, r := classifyAuthError(err)
			reason = r
			outcome = "error"

			// set rpc.grpc.status_code also for failed auth
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
}

// SessionStreamServerInterceptor applies identity resolution for streaming RPCs.
func SessionStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if info.FullMethod == reflectionMethod {
			return handler(srv, stream)
		}

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

		outcome := "success"
		reason := "ok"

		if err != nil {
			code, r := classifyAuthError(err)
			reason = r
			outcome = "error"

			span.SetAttributes(
				attribute.Int("rpc.grpc.status_code", int(code)),
			)

			observeIdentityResolution(ctx, source, outcome, reason, start)

			return status.Error(code, err.Error())
		}

		observeIdentityResolution(ctx, source, outcome, reason, start)

		span.SetAttributes(attribute.String("enduser.id", userID))
		span.SetStatus(otelcodes.Ok, "user identity resolved")

		ctx = session.WithUserID(ctx, userID)

		err = handler(srv, wrapStreamWithContext(ctx, stream))

		grpcCode := grpcCodes.OK
		if err != nil {
			grpcCode = status.Code(err)
		}

		span.SetAttributes(
			attribute.Int("rpc.grpc.status_code", int(grpcCode)),
		)

		return err
	}
}

// resolveUserIdentity orchestrates the identity resolution flow.
func resolveUserIdentity(ctx context.Context, span trace.Span) (string, string, error) {
	sess, err := loadSession(ctx, span)
	if err != nil {
		return "", "unknown", err
	}

	// 1. Metadata (primary)
	if userID, ok, err := resolveFromMetadata(ctx, span, sess); err != nil {
		return "", "metadata", err
	} else if ok {
		return userID, "metadata", nil
	}

	// 2. Session fallback
	if userID, ok := resolveFromSession(span, sess); ok {
		return userID, "session", nil
	}

	// 3. Context fallback
	userID, err := resolveFromContext(ctx, span)
	if err != nil {
		return "", "context", err
	}

	return userID, "context", nil
}

// loadSession reads Ory session from context.
func loadSession(ctx context.Context, span trace.Span) (*ory.Session, error) {
	sess, err := session.GetSession(ctx)
	if err != nil && !errors.Is(err, session.ErrSessionNotFound) {
		wrap := &SessionLoadError{Err: err}
		span.RecordError(wrap)
		span.SetStatus(otelcodes.Error, wrap.Error())
		return nil, wrap
	}
	return sess, nil
}

// resolveFromMetadata reads user-id from gRPC metadata and validates it against the session.
func resolveFromMetadata(ctx context.Context, span trace.Span, sess *ory.Session) (string, bool, error) {
	userID, err := extractUserIDFromIncomingMetadata(ctx)
	if err != nil {
		if errors.Is(err, ErrServerMissingMetadata) || errors.Is(err, ErrServerMissingUserID) {
			return "", false, nil
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return "", false, err
	}

	span.SetAttributes(attribute.String("auth.user_id.metadata", userID))

	if sess != nil {
		if identity, ok := sess.GetIdentityOk(); ok && identity != nil {
			sessID := identity.GetId()
			if sessID != "" && sessID != userID {
				mismatch := UserIDMismatchError{MetadataID: userID, SessionID: sessID}
				span.RecordError(mismatch)
				span.SetStatus(otelcodes.Error, mismatch.Error())
				return "", false, mismatch
			}
		}
	}

	span.SetAttributes(
		attribute.String("auth.user_id.source", "metadata"),
		attribute.String("auth.user_id", userID),
	)

	return userID, true, nil
}

func resolveFromSession(span trace.Span, sess *ory.Session) (string, bool) {
	if sess == nil {
		return "", false
	}

	identity, ok := sess.GetIdentityOk()
	if !ok || identity == nil {
		return "", false
	}

	sessID := identity.GetId()
	if sessID == "" {
		return "", false
	}

	span.SetAttributes(
		attribute.String("auth.user_id.source", "session"),
		attribute.String("auth.user_id", sessID),
	)

	return sessID, true
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

// extract user-id from gRPC metadata
func extractUserIDFromIncomingMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ErrServerMissingMetadata
	}

	values := md.Get(session.ContextUserIDKey.String())
	if len(values) == 0 {
		return "", ErrServerMissingUserID
	}

	userID := strings.TrimSpace(values[0])
	if userID == "" {
		return "", ErrServerMissingUserID
	}

	return userID, nil
}

// --- Stream Wrapper ----

type sessionWrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func wrapStreamWithContext(ctx context.Context, stream grpc.ServerStream) grpc.ServerStream {
	return &sessionWrappedServerStream{ServerStream: stream, ctx: ctx}
}

func (s *sessionWrappedServerStream) Context() context.Context {
	return s.ctx
}

// --- Error Mapping ----

func toStatusErr(err error) error {
	code, _ := classifyAuthError(err)
	return status.Error(code, err.Error())
}

func classifyAuthError(err error) (grpcCodes.Code, string) {
	var mismatchErr *UserIDMismatchError
	var sessionLoadErr *SessionLoadError

	switch {
	case errors.As(err, &mismatchErr):
		return grpcCodes.PermissionDenied, "user_id_mismatch"

	case errors.As(err, &sessionLoadErr):
		return grpcCodes.Internal, "session_load_error"

	case errors.Is(err, ErrServerMissingMetadata):
		return grpcCodes.Unauthenticated, "missing_metadata"

	case errors.Is(err, ErrServerMissingUserID):
		return grpcCodes.Unauthenticated, "missing_user_id"

	default:
		return grpcCodes.Internal, "internal_error"
	}
}

// --- Metrics (with exemplars) ----

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

// --- Utils ----

func splitFullMethodName(full string) (string, string) {
	parts := strings.Split(full, "/")
	if len(parts) != 3 {
		return "", ""
	}
	return parts[1], parts[2]
}
