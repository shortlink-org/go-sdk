package session_interceptor

import (
	"context"
	"errors"

	"github.com/shortlink-org/go-sdk/auth/session"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const reflectionMethod = "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo"

// SessionUnaryServerInterceptor returns a unary server interceptor that ensures
// every incoming unary RPC request carries a valid user identifier.
//
// Identity resolution rules:
//  1. Extract user-id from incoming metadata (mandatory)
//  2. Load Ory session if present
//  3. Validate that metadata user-id matches session identity ID (if session exists)
//  4. Inject user-id into context for downstream handlers
//
// All identity resolution is traced via OpenTelemetry.
func SessionUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Skip reflection calls (grpcurl, tools, IDEs)
		if info.FullMethod == reflectionMethod {
			return handler(ctx, req)
		}

		tracer := otel.Tracer("session.interceptor.server")

		ctx, span := tracer.Start(ctx, "ResolveUserIdentity")
		defer span.End()

		userID, err := resolveUserIdentity(ctx, span)
		if err != nil {
			return nil, err
		}

		// Inject user-id into context for downstream business logic
		ctx = session.WithUserID(ctx, userID)

		span.SetStatus(codes.Ok, "user identity resolved")

		return handler(ctx, req)
	}
}

// SessionStreamServerInterceptor returns a stream server interceptor that ensures
// every incoming streaming RPC request carries a valid user identifier.
//
// Uses the same identity resolution flow as the unary interceptor, but applied
// to streaming RPCs. The stream wrapper overrides Context() so that downstream
// handlers see the resolved identity.
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

		baseCtx := stream.Context()
		tracer := otel.Tracer("session.interceptor.server")

		ctx, span := tracer.Start(baseCtx, "ResolveUserIdentityStream")
		defer span.End()

		userID, err := resolveUserIdentity(ctx, span)
		if err != nil {
			return err
		}

		// Inject user-id into context
		ctx = session.WithUserID(ctx, userID)

		span.SetStatus(codes.Ok, "user identity resolved")

		// Wrap stream so context is visible to handlers
		wrapped := wrapStreamWithContext(ctx, stream)

		return handler(srv, wrapped)
	}
}

// resolveUserIdentity extracts and validates user identity from metadata and session.
func resolveUserIdentity(ctx context.Context, span trace.Span) (string, error) {
	// Extract required user-id from metadata
	userID, err := extractUserIDFromIncomingMetadata(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return "", err
	}

	span.SetAttributes(attribute.String("auth.user_id.metadata", userID))

	// Attempt to load Ory session; ignore ErrSessionNotFound
	sess, err := session.GetSession(ctx)
	if err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			// wrap session loading error
			wrap := &SessionLoadError{Err: err}
			span.RecordError(wrap)
			span.SetStatus(codes.Error, wrap.Error())

			return "", wrap
		}
		// no session — OK
	}

	orySession := sess

	// If a session exists, validate identity consistency
	if orySession != nil {
		if identity, hasIdentity := orySession.GetIdentityOk(); hasIdentity && identity != nil {
			sessID := identity.GetId()
			span.SetAttributes(attribute.String("auth.user_id.session", sessID))

			if sessID != "" && sessID != userID {
				mismatch := UserIDMismatchError{MetadataID: userID, SessionID: sessID}

				span.AddEvent("identity_mismatch")
				span.RecordError(mismatch)
				span.SetStatus(codes.Error, mismatch.Error())

				return "", mismatch
			}
		}
	}

	return userID, nil
}

// extractUserIDFromIncomingMetadata extracts user-id from incoming metadata.
// This value is mandatory. If missing or empty, the request is rejected.
func extractUserIDFromIncomingMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ErrServerMissingMetadata
	}

	vals := md.Get(session.ContextUserIDKey.String())
	if len(vals) == 0 || vals[0] == "" {
		return "", ErrServerMissingUserID
	}

	return vals[0], nil
}

// sessionWrappedServerStream wraps a ServerStream to override Context()
// with an enriched context containing the resolved user identity.
//
//nolint:containedctx // context is required for gRPC stream wrapper pattern
type sessionWrappedServerStream struct {
	grpc.ServerStream

	ctx context.Context
}

// wrapStreamWithContext wraps a ServerStream with a context containing
// the resolved user identity.
//
//nolint:ireturn // required by gRPC interface
func wrapStreamWithContext(ctx context.Context, stream grpc.ServerStream) grpc.ServerStream {
	return &sessionWrappedServerStream{
		ServerStream: stream,
		ctx:          ctx,
	}
}

func (s *sessionWrappedServerStream) Context() context.Context {
	return s.ctx
}
