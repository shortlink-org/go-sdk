// Package sessioninterceptor provides gRPC interceptors for JWT token propagation.
// This package forwards JWT tokens from HTTP requests to downstream gRPC services,
// enabling end-to-end authentication through the service mesh.
package sessioninterceptor

import (
	"context"

	"github.com/shortlink-org/go-sdk/auth/session"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	// authorizationKey is the gRPC metadata key for the JWT token.
	// Must be lowercase for gRPC metadata.
	authorizationKey = "authorization"

	// userIDKey is the gRPC metadata key for the user ID.
	userIDKey = "user-id"

	// initialPairsCapacity is the initial capacity for metadata pairs slice.
	initialPairsCapacity = 4
)

type contextKey struct{ name string }

// ContextAuthorizationKey is used to store the original Authorization header in context.
var ContextAuthorizationKey = &contextKey{"authorization"}

var tracerClient = otel.Tracer("session.interceptor.client")

// WithAuthorization stores the Authorization header value in context.
// Call this in your HTTP middleware to make it available for gRPC calls.
func WithAuthorization(ctx context.Context, authHeader string) context.Context {
	return context.WithValue(ctx, ContextAuthorizationKey, authHeader)
}

// GetAuthorization retrieves the Authorization header from context.
func GetAuthorization(ctx context.Context) string {
	if v, ok := ctx.Value(ContextAuthorizationKey).(string); ok {
		return v
	}

	return ""
}

// attachAuthMetadata injects JWT token and user-id into outgoing gRPC metadata.
func attachAuthMetadata(ctx context.Context) (context.Context, error) {
	ctx, span := tracerClient.Start(ctx, "AttachAuthMetadata")
	defer span.End()

	// Get Authorization header from context
	auth := GetAuthorization(ctx)

	// Get user ID from JWT claims
	userID, _ := session.GetUserID(ctx)

	// Build metadata
	pairs := make([]string, 0, initialPairsCapacity)

	if auth != "" {
		pairs = append(pairs, authorizationKey, auth)

		span.SetAttributes(attribute.Bool("auth.token_present", true))
	} else {
		span.SetAttributes(attribute.Bool("auth.token_present", false))
	}

	if userID != "" {
		pairs = append(pairs, userIDKey, userID)
		span.SetAttributes(attribute.String("auth.user_id", userID))
	}

	if len(pairs) == 0 {
		span.SetStatus(codes.Ok, "no auth metadata to attach")

		return ctx, nil
	}

	span.SetStatus(codes.Ok, "auth metadata attached")

	return metadata.AppendToOutgoingContext(ctx, pairs...), nil
}

// SessionUnaryClientInterceptor attaches JWT token and user-id to outgoing metadata for unary calls.
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
		ctx, err := attachAuthMetadata(ctx)
		if err != nil {
			return err
		}

		return invoker(ctx, method, req, resp, clientConn, opts...)
	}
}

// SessionStreamClientInterceptor attaches JWT token and user-id to outgoing metadata for streaming calls.
func SessionStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		clientConn *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx, err := attachAuthMetadata(ctx)
		if err != nil {
			return nil, err
		}

		return streamer(ctx, desc, clientConn, method, opts...)
	}
}
