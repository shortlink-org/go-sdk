package consistency

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryServerInterceptor gates the handler's reads on the caller's watermark,
// and returns its own in the trailer.
//
// A request that arrives with a token is scoped to it: reads may use a replica
// once that replica has replayed past it, and go to the primary until then. A
// request that arrives without one is still scoped, so that its reads can use a
// replica at all — an unscoped request falls back to the primary for everything.
func UnaryServerInterceptor(guarantee Consistency) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if guarantee == nil || shouldSkipMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		handlerCtx := scope(ctx, guarantee)

		resp, err := handler(handlerCtx, req)

		// Set the trailer even when the handler failed: a handler can return
		// an error after part of its work has committed, and the caller still
		// must not read behind it.
		setWatermarkTrailer(handlerCtx, guarantee, func(md metadata.MD) {
			//nolint:errcheck // a trailer that cannot be set means the RPC is already over
			_ = grpc.SetTrailer(ctx, md)
		})

		return resp, err
	}
}

// StreamServerInterceptor is the streaming counterpart.
func StreamServerInterceptor(guarantee Consistency) grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if guarantee == nil || shouldSkipMethod(info.FullMethod) {
			return handler(srv, stream)
		}

		handlerCtx := scope(stream.Context(), guarantee)

		err := handler(srv, &wrappedServerStream{ServerStream: stream, wrappedCtx: handlerCtx})

		setWatermarkTrailer(handlerCtx, guarantee, stream.SetTrailer)

		return err
	}
}

// scope applies an incoming watermark, or installs a fresh one when the
// request carries none.
func scope(ctx context.Context, guarantee Consistency) context.Context {
	incoming, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return guarantee.Scope(ctx)
	}

	token := tokenFrom(incoming)
	if token == "" {
		return guarantee.Scope(ctx)
	}

	return guarantee.Apply(ctx, token)
}

// setWatermarkTrailer hands the handler's own watermark back to the caller,
// and only when the handler wrote — Watermark reports that without a round
// trip, so a read-only RPC pays nothing.
func setWatermarkTrailer(ctx context.Context, guarantee Consistency, set func(metadata.MD)) {
	token, ok, err := guarantee.Watermark(ctx)
	if err != nil || !ok {
		return
	}

	set(metadata.Pairs(MetadataKey, token))
}

// wrappedServerStream overrides the context a streaming handler sees.
//
//nolint:containedctx // required by the grpc stream context override pattern
type wrappedServerStream struct {
	grpc.ServerStream

	wrappedCtx context.Context
}

func (s *wrappedServerStream) Context() context.Context {
	return s.wrappedCtx
}
