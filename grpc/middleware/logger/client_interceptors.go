// Package grpc_logger provides client-side logging interceptors for gRPC.
package grpc_logger

import (
	"context"
	"log/slog"
	"path"
	"time"

	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// UnaryClientInterceptor returns a new unary client interceptor that optionally
// logs the execution of external gRPC calls.
func UnaryClientInterceptor(log logger.Logger) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		startTime := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(startTime)

		if span := trace.SpanFromContext(ctx); span.IsRecording() {
			if msg, ok := req.(proto.Message); ok {
				span.SetAttributes(attribute.String("rpc.request", string(proto.MessageName(msg))))
			}

			if msg, ok := reply.(proto.Message); ok {
				span.SetAttributes(attribute.String("rpc.response", string(proto.MessageName(msg))))
			}
		}

		fields := []slog.Attr{
			slog.String("grpc.service", path.Dir(method)[1:]),
			slog.String("grpc.method", path.Base(method)),
			slog.String("code", status.Code(err).String()),
			slog.Int64("duration (mks)", duration.Microseconds()),
		}

		if err != nil {
			printLog(ctx, log, err, fields...)
		}

		return err
	}
}

// StreamClientInterceptor returns a new streaming client interceptor that optionally
// logs the execution of external gRPC calls.
func StreamClientInterceptor(log logger.Logger) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		clientStream, err := streamer(ctx, desc, cc, method, opts...)

		fields := []slog.Attr{
			slog.String("grpc.service", path.Dir(method)[1:]),
			slog.String("grpc.method", path.Base(method)),
			slog.String("code", status.Code(err).String()),
		}

		if err != nil {
			printLog(ctx, log, err, fields...)
		}

		return clientStream, err
	}
}
