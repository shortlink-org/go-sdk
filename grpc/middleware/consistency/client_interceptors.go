package consistency

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryClientInterceptor sends the caller's watermark with the request, and
// takes the callee's back from the trailer.
//
// The outgoing half means the callee reads no earlier than the caller wrote.
// The returning half means that once the callee has written on our behalf, our
// own later reads do not run behind it.
//
// A failure to read the watermark is not a failure of the call. Turning a
// consistency optimization into an availability regression is the wrong trade:
// without the token the callee simply reads from the primary, which is what it
// did before any of this existed.
func UnaryClientInterceptor(guarantee Consistency) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req any,
		reply any,
		clientConn *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if guarantee == nil {
			return invoker(ctx, method, req, reply, clientConn, opts...)
		}

		callCtx := ctx

		token, ok, err := guarantee.Watermark(ctx)
		if err == nil && ok {
			callCtx = attach(ctx, token)
		}

		var trailer metadata.MD

		opts = append(opts, grpc.Trailer(&trailer))

		err = invoker(callCtx, method, req, reply, clientConn, opts...)

		// Read the trailer even on error: a call can fail after the write it
		// made has committed, and forgetting that write is exactly the case
		// this is here for.
		returned := tokenFrom(trailer)
		if returned != "" {
			guarantee.Observe(ctx, returned)
		}

		return err
	}
}

// StreamClientInterceptor sends the caller's watermark with the stream.
//
// It does not read the trailer back. A stream's trailer only exists once the
// stream is done, which is somewhere inside the caller's own loop rather than
// here, so observing it would mean wrapping every RecvMsg to watch for the end.
// For a streaming RPC that writes on the caller's behalf, take the watermark
// back through the response message instead, and hand it to Consistency.Observe
// yourself.
func StreamClientInterceptor(guarantee Consistency) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		clientConn *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if guarantee == nil {
			return streamer(ctx, desc, clientConn, method, opts...)
		}

		token, ok, err := guarantee.Watermark(ctx)
		if err == nil && ok {
			ctx = attach(ctx, token)
		}

		return streamer(ctx, desc, clientConn, method, opts...)
	}
}
