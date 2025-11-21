// Package session_interceptor provides gRPC interceptors for session management.
//

package session_interceptor

import (
	"context"
	"errors"

	ory "github.com/ory/client-go"
	"github.com/shortlink-org/go-sdk/auth/session"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const userIDKey = "user-id"

var tracer = otel.Tracer("session.interceptor.client")

// attachUserMetadata resolves and injects a stable user-id into outgoing gRPC metadata.
// Priority: metadata → session.identity → context fallback.
func attachUserMetadata(ctx context.Context) (context.Context, error) {
	ctx, span := tracer.Start(ctx, "ResolveOutgoingUserID")
	defer span.End()

	// Load session once to avoid double calls
	sess, sessErr := session.GetSession(ctx)
	if sessErr != nil && !errors.Is(sessErr, session.ErrSessionNotFound) {
		err := &SessionLoadError{Err: sessErr}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return nil, err
	}

	// Try metadata first if available and valid
	uid, foundInMetadata, err := extractUserIDFromMetadata(ctx, sess)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return nil, err
	}

	if foundInMetadata {
		span.SetAttributes(attribute.String("auth.user_id.source", "metadata"))
		span.SetAttributes(attribute.String("auth.user_id", uid))
		span.SetStatus(codes.Ok, "user-id resolved from metadata")

		return metadata.AppendToOutgoingContext(ctx, userIDKey, uid), nil
	}

	// Try session identity if available
	uid, foundInSession := extractUserIDFromSession(sess)
	if foundInSession {
		span.SetAttributes(attribute.String("auth.user_id.source", "session"))
		span.SetAttributes(attribute.String("auth.user_id", uid))
		span.SetStatus(codes.Ok, "user-id resolved from session")

		return metadata.AppendToOutgoingContext(ctx, userIDKey, uid), nil
	}

	// Fallback to context-injected user-id
	uid, err = extractUserIDFromContext(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return nil, err
	}

	span.SetAttributes(attribute.String("auth.user_id.source", "context"))
	span.SetAttributes(attribute.String("auth.user_id", uid))
	span.SetStatus(codes.Ok, "user-id resolved from context")

	return metadata.AppendToOutgoingContext(ctx, userIDKey, uid), nil
}

// extractUserIDFromMetadata reads user-id from outgoing metadata and validates against session.
func extractUserIDFromMetadata(ctx context.Context, sess *ory.Session) (string, bool, error) {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return "", false, nil
	}

	vals := md.Get(userIDKey)
	if len(vals) == 0 {
		return "", false, nil
	}

	uid := vals[0]
	if uid == "" {
		return "", false, nil
	}

	// Validate metadata UID matches session UID if session exists
	err := validateUserIDConsistency(sess, uid)
	if err != nil {
		return "", false, err
	}

	return uid, true, nil
}

// extractUserIDFromSession derives user-id from session identity.
func extractUserIDFromSession(sess *ory.Session) (string, bool) {
	if sess == nil {
		return "", false
	}

	identity, hasIdentity := sess.GetIdentityOk()
	if !hasIdentity || identity == nil {
		return "", false
	}

	uid := identity.GetId()
	if uid == "" {
		return "", false
	}

	return uid, true
}

// extractUserIDFromContext fallback resolver: derives user-id from context.
func extractUserIDFromContext(ctx context.Context) (string, error) {
	uid, err := session.GetUserID(ctx)
	if err != nil {
		return "", &UserIDNotFoundError{Err: err}
	}

	if uid == "" {
		return "", ErrEmptyUserID
	}

	return uid, nil
}

// validateUserIDConsistency ensures metadata user-id matches session user-id.
func validateUserIDConsistency(sess *ory.Session, metadataUID string) error {
	if sess == nil {
		return nil
	}

	identity, hasIdentity := sess.GetIdentityOk()
	if !hasIdentity || identity == nil {
		return nil
	}

	sessUID := identity.GetId()
	if sessUID == "" {
		return nil
	}

	if metadataUID != sessUID {
		return UserIDMismatchError{MetadataID: metadataUID, SessionID: sessUID}
	}

	return nil
}

// SessionUnaryClientInterceptor attaches user-id to outgoing metadata for unary calls.
func SessionUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req any,
		resp any,
		clientConn *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx, err := attachUserMetadata(ctx)
		if err != nil {
			return err
		}

		return invoker(ctx, method, req, resp, clientConn, opts...)
	}
}

// SessionStreamClientInterceptor attaches user-id to outgoing metadata for streaming calls.
func SessionStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		clientConn *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx, err := attachUserMetadata(ctx)
		if err != nil {
			return nil, err
		}

		return streamer(ctx, desc, clientConn, method, opts...)
	}
}
