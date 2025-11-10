package flight_trace

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/spf13/viper"

	"github.com/shortlink-org/go-sdk/flight_trace"
	"github.com/shortlink-org/go-sdk/logger"
)

// DebugTraceMiddleware triggers a Go Flight Recorder dump when:
// - "X-Debug-Trace: true" header is present, OR
// - request latency exceeds configured threshold (FLIGHT_TRACE_LATENCY_THRESHOLD).
// The dump filename is attached to the current trace span.
func DebugTraceMiddleware(fr *flight_trace.Recorder, log logger.Logger) func(http.Handler) http.Handler {
	// Default threshold (can be overridden via ENV)
	viper.SetDefault("FLIGHT_TRACE_LATENCY_THRESHOLD", "1s")

	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			if fr == nil {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			shouldDump := r.Header.Get("X-Debug-Trace") == "true"

			next.ServeHTTP(w, r)

			latency := time.Since(start)
			if latency > viper.GetDuration("FLIGHT_TRACE_LATENCY_THRESHOLD") {
				shouldDump = true
			}

			if shouldDump {
				fileName := "trace-" + uuid.NewString() + ".out"

				// Attach dump filename to current OpenTelemetry span
				if span := trace.SpanFromContext(r.Context()); span != nil && span.IsRecording() {
					span.SetAttributes(attribute.String("flight_trace.file", fileName))
				}

				go func() {
					fr.DumpToFileAsync(fileName)
					log.InfoWithContext(r.Context(), "flight recorder dump triggered",
						slog.String("file", fileName),
						slog.Duration("latency", latency),
						slog.String("remote", r.RemoteAddr),
						slog.String("method", r.Method),
						slog.String("uri", r.RequestURI),
					)
				}()
			}
		}

		return http.HandlerFunc(fn)
	}
}
