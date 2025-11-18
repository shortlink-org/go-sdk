package logger_middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/shortlink-org/go-sdk/logger"
)

type chilogger struct {
	log logger.Logger
}

// Logger returns a new Zap Middleware handler.
func Logger(log logger.Logger) func(next http.Handler) http.Handler {
	return chilogger{
		log: log,
	}.middleware
}

func (c chilogger) middleware(next http.Handler) http.Handler {
	handlerFunc := func(writer http.ResponseWriter, request *http.Request) {
		start := time.Now()
		wrappedWriter := middleware.NewWrapResponseWriter(writer, request.ProtoMajor)

		defer func() {
			latency := time.Since(start)

			fields := []slog.Attr{
				slog.Int("status", wrappedWriter.Status()),
				slog.Duration("took", latency),
				slog.String("remote", request.RemoteAddr),
				slog.String("request", request.RequestURI),
				slog.String("method", request.Method),
			}

			switch {
			case wrappedWriter.Status() >= http.StatusInternalServerError:
				c.log.ErrorWithContext(request.Context(), "request completed", fields...)
			case wrappedWriter.Status() >= http.StatusBadRequest:
				c.log.WarnWithContext(request.Context(), "request completed", fields...)
			default:
				c.log.InfoWithContext(request.Context(), "request completed", fields...)
			}
		}()

		next.ServeHTTP(wrappedWriter, request)
	}

	return http.HandlerFunc(handlerFunc)
}
