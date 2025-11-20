package flight_trace

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/flight_trace"
	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// DebugTraceMiddleware triggers a Go Flight Recorder dump when:
// - "X-Debug-Trace: true" header is present, OR
// - request latency exceeds configured threshold (FLIGHT_TRACE_LATENCY_THRESHOLD).
// The dump filename is attached to the current trace span.
func DebugTraceMiddleware(recorder *flight_trace.Recorder, loggerInstance logger.Logger, cfg *config.Config) func(http.Handler) http.Handler {
	// Default threshold (can be overridden via ENV)
	cfg.SetDefault("FLIGHT_TRACE_LATENCY_THRESHOLD", "1s")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if recorder == nil {
				next.ServeHTTP(writer, request)
				return
			}

			start := time.Now()
			shouldDump := request.Header.Get("X-Debug-Trace") == "true"

			next.ServeHTTP(writer, request)

			latency := time.Since(start)
			if latency > cfg.GetDuration("FLIGHT_TRACE_LATENCY_THRESHOLD") {
				shouldDump = true
			}

			if shouldDump {
				fileName := "trace-" + uuid.NewString() + ".out"
				ctx := request.Context()

				if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
					span.SetAttributes(attribute.String("flight_trace.file", fileName))
				}

				job := &dumpJob{
					recorder: recorder,
					logger:   loggerInstance,
					fileName: fileName,
					latency:  latency,
					meta:     captureRequestMeta(request),
				}
				go job.run(ctx)
			}
		})
	}
}

type requestMeta struct {
	remoteAddr string
	method     string
	uri        string
}

func captureRequestMeta(request *http.Request) requestMeta {
	return requestMeta{
		remoteAddr: request.RemoteAddr,
		method:     request.Method,
		uri:        request.RequestURI,
	}
}

type dumpJob struct {
	recorder *flight_trace.Recorder
	logger   logger.Logger
	fileName string
	latency  time.Duration
	meta     requestMeta
}

func (job *dumpJob) run(ctx context.Context) {
	job.recorder.DumpToFileAsync(job.fileName)
	job.logger.InfoWithContext(ctx, "flight recorder dump triggered",
		slog.String("file", job.fileName),
		slog.Duration("latency", job.latency),
		slog.String("remote", job.meta.remoteAddr),
		slog.String("method", job.meta.method),
		slog.String("uri", job.meta.uri),
	)
}
