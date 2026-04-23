package logger_middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/logger"
)

type chilogger struct {
	log logger.Logger
}

func Logger(log logger.Logger) func(next http.Handler) http.Handler {
	return chilogger{log: log}.middleware
}

func (c chilogger) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		start := time.Now()

		// Preserve original writer but wrap it to intercept status + bytes written
		wrapped := middleware.NewWrapResponseWriter(rw, req.ProtoMajor)

		defer func() {
			latency := time.Since(start)
			status := wrapped.Status()
			bytes := wrapped.BytesWritten()

			// Recover panic if happened
			if rec := recover(); rec != nil {
				c.log.ErrorWithContext(
					req.Context(),
					"panic recovered",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
					slog.String("method", req.Method),
					slog.String("path", req.URL.Path),
				)
				http.Error(wrapped, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

				return
			}

			fields := []slog.Attr{
				slog.Int("status", status),
				slog.Int("bytes", bytes),
				slog.Int64("took_ms", latency.Milliseconds()),
				slog.String("remote", req.RemoteAddr),
				slog.String("method", req.Method),
				slog.String("path", req.URL.Path),
				slog.String("query", req.URL.RawQuery),
				slog.String("user_agent", req.UserAgent()),
				slog.String("referer", req.Referer()),
			}

			// Extract OTEL trace IDs if available
			span := trace.SpanFromContext(req.Context())
			if span != nil && span.IsRecording() {
				spanCtx := span.SpanContext()
				if spanCtx.HasTraceID() {
					fields = append(fields,
						slog.String("trace_id", spanCtx.TraceID().String()),
					)
				}

				if spanCtx.HasSpanID() {
					fields = append(fields,
						slog.String("span_id", spanCtx.SpanID().String()),
					)
				}
			}

			// Log level depending on status
			switch {
			case status >= http.StatusInternalServerError:
				c.log.ErrorWithContext(req.Context(), "request completed", fields...)
			case status >= http.StatusBadRequest:
				c.log.WarnWithContext(req.Context(), "request completed", fields...)
			default:
				c.log.InfoWithContext(req.Context(), "request completed", fields...)
			}
		}()

		next.ServeHTTP(wrapped, req)
	})
}
