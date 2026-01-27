package authforward

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

var tracer = otel.Tracer("authforward")

// =============================================================================
// Server Interceptors - Capture incoming token and store in context
// =============================================================================

// UnaryServerInterceptor captures the Authorization header from incoming metadata
// and stores it in context for downstream forwarding.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		ctx = captureToken(ctx)

		return handler(ctx, req)
	}
}

// StreamServerInterceptor captures the Authorization header from incoming metadata
// and stores it in context for downstream forwarding.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := captureToken(stream.Context())

		return handler(srv, &wrappedServerStream{ServerStream: stream, wrappedCtx: ctx})
	}
}

// captureToken extracts token from incoming metadata and stores in context.
func captureToken(ctx context.Context) context.Context {
	token := TokenFromIncomingMetadata(ctx)
	if token != "" {
		ctx = WithToken(ctx, token)
	}

	return ctx
}

// =============================================================================
// Client Interceptors - Forward token from context to outgoing metadata
// =============================================================================

// UnaryClientInterceptor forwards the Authorization token from context
// to outgoing gRPC metadata for downstream services.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		conn *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx = forwardToken(ctx, method)

		return invoker(ctx, method, req, reply, conn, opts...)
	}
}

// StreamClientInterceptor forwards the Authorization token from context
// to outgoing gRPC metadata for downstream services.
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		conn *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx = forwardToken(ctx, method)

		return streamer(ctx, desc, conn, method, opts...)
	}
}

// forwardToken copies token from context to outgoing metadata.
func forwardToken(ctx context.Context, method string) context.Context {
	_, span := tracer.Start(ctx, "authforward.ForwardToken",
		trace.WithAttributes(attribute.String("rpc.method", method)),
	)
	defer span.End()

	// Get token from context (captured by server interceptor)
	token := TokenFromContext(ctx)
	if token == "" {
		span.SetAttributes(attribute.Bool("auth.token_present", false))
		span.SetStatus(codes.Ok, "no token to forward")

		return ctx
	}

	if existing := TokenFromOutgoingMetadata(ctx); existing != "" {
		span.SetAttributes(attribute.Bool("auth.token_replaced", true))
	}

	span.SetAttributes(attribute.Bool("auth.token_present", true))
	span.SetStatus(codes.Ok, "token forwarded")

	return SetOutgoingToken(ctx, token)
}

// =============================================================================
// Stream Wrapper
// =============================================================================

//nolint:containedctx // Required for grpc stream context override pattern
type wrappedServerStream struct {
	grpc.ServerStream

	wrappedCtx context.Context
}

func (wrapper *wrappedServerStream) Context() context.Context {
	return wrapper.wrappedCtx
}
