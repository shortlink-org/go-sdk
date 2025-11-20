package flight_trace

import (
	"context"
	"log/slog"
	"path"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/flight_trace"
	"github.com/shortlink-org/go-sdk/logger"
)

const debugTraceKey = "X-DEBUG-TRACE"

// UnaryServerInterceptor records Flight Recorder dumps based on conditions:
// - Incoming metadata contains "X-DEBUG-TRACE: true"
// - The request latency exceeds FLIGHT_TRACE_LATENCY_THRESHOLD
func UnaryServerInterceptor(fr *flight_trace.Recorder, log logger.Logger, cfg *config.Config) grpc.UnaryServerInterceptor {
	cfg.SetDefault("FLIGHT_TRACE_LATENCY_THRESHOLD", "1s")

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if fr == nil {
			return handler(ctx, req)
		}

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		md, _ := metadata.FromIncomingContext(ctx)
		vals := md.Get(debugTraceKey)
		shouldDump := len(vals) > 0 && vals[0] == "true"

		threshold := cfg.GetDuration("FLIGHT_TRACE_LATENCY_THRESHOLD")
		shouldDump = shouldDump || duration > threshold

		if shouldDump {
			fileName := "grpc-" + uuid.NewString() + ".out"

			if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
				span.SetAttributes(attribute.String("flight_trace.file", fileName))
			}

			go func() {
				fr.DumpToFileAsync(fileName)
				log.InfoWithContext(ctx, "flight recorder dump triggered",
					slog.String("file", fileName),
					slog.String("grpc.service", path.Dir(info.FullMethod)[1:]),
					slog.String("grpc.method", path.Base(info.FullMethod)),
					slog.String("code", status.Code(err).String()),
					slog.Duration("latency", duration),
				)
			}()
		}

		return resp, err
	}
}

// StreamServerInterceptor is similar to UnaryServerInterceptor but for streaming RPCs.
func StreamServerInterceptor(fr *flight_trace.Recorder, log logger.Logger, cfg *config.Config) grpc.StreamServerInterceptor {
	cfg.SetDefault("FLIGHT_TRACE_LATENCY_THRESHOLD", "1s")

	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if fr == nil {
			return handler(srv, stream)
		}

		start := time.Now()
		err := handler(srv, stream)
		duration := time.Since(start)

		md, _ := metadata.FromIncomingContext(stream.Context())
		vals := md.Get(debugTraceKey)
		shouldDump := len(vals) > 0 && vals[0] == "true"

		threshold := cfg.GetDuration("FLIGHT_TRACE_LATENCY_THRESHOLD")
		shouldDump = shouldDump || duration > threshold

		if shouldDump {
			fileName := "grpc-" + uuid.NewString() + ".out"

			if span := trace.SpanFromContext(stream.Context()); span != nil && span.IsRecording() {
				span.SetAttributes(attribute.String("flight_trace.file", fileName))
			}

			go func() {
				fr.DumpToFileAsync(fileName)
				log.InfoWithContext(stream.Context(), "flight recorder dump triggered (stream)",
					slog.String("file", fileName),
					slog.String("grpc.service", path.Dir(info.FullMethod)[1:]),
					slog.String("grpc.method", path.Base(info.FullMethod)),
					slog.String("code", status.Code(err).String()),
					slog.Duration("latency", duration),
				)
			}()
		}

		return err
	}
}
