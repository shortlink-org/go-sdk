package grpc_logger

import (
	"context"
	"log/slog"
	"path"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/shortlink-org/go-sdk/logger"
)

// UnaryServerInterceptor returns a new unary server interceptors that adds zap.Logger to the context.
func UnaryServerInterceptor(log logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		startTime := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(startTime)

		if span := trace.SpanFromContext(ctx); span.IsRecording() {
			if msg, ok := req.(proto.Message); ok {
				span.SetAttributes(attribute.String("rpc.request", string(proto.MessageName(msg))))
			}
			if msg, ok := resp.(proto.Message); ok {
				span.SetAttributes(attribute.String("rpc.response", string(proto.MessageName(msg))))
			}
		}

		fields := []slog.Attr{
			slog.String("grpc.service", path.Dir(info.FullMethod)[1:]),
			slog.String("grpc.method", path.Base(info.FullMethod)),
			slog.String("code", status.Code(err).String()),
			slog.Int64("duration (mks)", duration.Microseconds()),
		}

		if err != nil {
			printLog(ctx, log, err, fields...)
		}

		return resp, err
	}
}

// StreamServerInterceptor returns a new streaming server interceptor that adds zap.Logger to the context.
func StreamServerInterceptor(log logger.Logger) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		startTime := time.Now()
		wrapped := grpc_middleware.WrapServerStream(stream)

		err := handler(srv, wrapped)
		duration := time.Since(startTime)

		fields := []slog.Attr{
			slog.String("grpc.service", path.Dir(info.FullMethod)[1:]),
			slog.String("grpc.method", path.Base(info.FullMethod)),
			slog.String("code", status.Code(err).String()),
			slog.Int64("duration (mks)", duration.Microseconds()),
		}

		if err != nil {
			printLog(wrapped.Context(), log, err, fields...)
		}

		return err
	}
}
