package session_interceptor

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/shortlink-org/go-sdk/auth/session"
)

const userIDKey = "user-id"

// predefined static errors (required by goerr113)
var (
	ErrEmptyUserID        = errors.New("attachUserMetadata: empty user id")
	ErrMissingUserID      = errors.New("attachUserMetadata: failed to get user id")
	ErrFailedToGetSession = errors.New("attachUserMetadata: failed to get session")
)

// attachUserMetadata resolves the user identifier and attaches it to outgoing gRPC metadata.
//
// Algorithm:
//  1. Try to get the full Ory session from context via session.GetSession.
//     - If an error occurs other than ErrSessionNotFound, it bubbles up.
//  2. If the session exists and carries an identity, attach that identity ID under "user-id".
//  3. If the session is missing (or does not include identity information), fall back to the user-id
//     stored separately in context.
//     - If user-id is also missing or empty, return an error.
//  4. Return a new context containing metadata with the resolved user identifier.
func attachUserMetadata(ctx context.Context) (context.Context, error) {
	sess, err := session.GetSession(ctx)
	if err != nil && !errors.Is(err, session.ErrSessionNotFound) {
		return nil, fmt.Errorf("%w: %w", ErrFailedToGetSession, err)
	}

	if sess != nil {
		if identity, ok := sess.GetIdentityOk(); ok && identity != nil {
			if identityID := identity.GetId(); identityID != "" {
				return metadata.AppendToOutgoingContext(ctx, userIDKey, identityID), nil
			}
		}
	}

	userID, err := session.GetUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMissingUserID, err)
	}

	if userID == "" {
		return nil, ErrEmptyUserID
	}

	return metadata.AppendToOutgoingContext(ctx, userIDKey, userID), nil
}

// SessionUnaryClientInterceptor adds user-id metadata to each unary gRPC call.
func SessionUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req any,
		resp any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx, err := attachUserMetadata(ctx)
		if err != nil {
			return err
		}

		return invoker(ctx, method, req, resp, cc, opts...)
	}
}

// SessionStreamClientInterceptor adds user-id metadata to each streaming gRPC call.
func SessionStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx, err := attachUserMetadata(ctx)
		if err != nil {
			return nil, err
		}

		return streamer(ctx, desc, cc, method, opts...)
	}
}
