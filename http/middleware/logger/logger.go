package logger_middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/shortlink-org/go-sdk/logger"

	"go.opentelemetry.io/otel/trace"
)

type chilogger struct {
	log logger.Logger
}

func Logger(log logger.Logger) func(next http.Handler) http.Handler {
	return chilogger{log: log}.middleware
}

func (c chilogger) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Preserve original writer but wrap it to intercept status + bytes written
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		defer func() {
			latency := time.Since(start)
			status := ww.Status()
			bytes := ww.BytesWritten()

			// Recover panic if happened
			if rec := recover(); rec != nil {
				c.log.ErrorWithContext(
					r.Context(),
					"panic recovered",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				http.Error(ww, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			fields := []slog.Attr{
				slog.Int("status", status),
				slog.Int("bytes", bytes),
				slog.Int64("took_ms", latency.Milliseconds()),
				slog.String("remote", r.RemoteAddr),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("query", r.URL.RawQuery),
				slog.String("user_agent", r.UserAgent()),
				slog.String("referer", r.Referer()),
			}

			// Extract OTEL trace IDs if available
			span := trace.SpanFromContext(r.Context())
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
				c.log.ErrorWithContext(r.Context(), "request completed", fields...)
			case status >= http.StatusBadRequest:
				c.log.WarnWithContext(r.Context(), "request completed", fields...)
			default:
				c.log.InfoWithContext(r.Context(), "request completed", fields...)
			}
		}()

		next.ServeHTTP(ww, r)
	})
}
