package span_middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TraceIDHeader is the header key for the trace id.
	TraceIDHeader = "trace-id"
)

type span struct{}

// Span is a middleware that adds a span to the response context.
func Span() func(next http.Handler) http.Handler {
	return span{}.middleware
}

func (s span) middleware(next http.Handler) http.Handler {
	handlerFunc := func(writer http.ResponseWriter, request *http.Request) {
		wrappedWriter := middleware.NewWrapResponseWriter(writer, request.ProtoMajor)

		// Get span from context once (span is created by otelhttp.NewHandler in server.go)
		// This middleware does NOT create new spans, only uses existing one
		span := trace.SpanFromContext(request.Context())

		// Check if "trace-id" already exists in the header
		if wrappedWriter.Header().Get(TraceIDHeader) == "" {
			// Inject traceId in response header
			if span.SpanContext().IsValid() {
				traceID := span.SpanContext().TraceID().String()
				if traceID != "" {
					wrappedWriter.Header().Set(TraceIDHeader, traceID)
				}
			}
		}

		next.ServeHTTP(wrappedWriter, request)

		// Set OTEL span status based on HTTP status code
		if span.IsRecording() {
			statusCode := wrappedWriter.Status()

			switch {
			case statusCode >= 200 && statusCode < 400:
				// 2xx, 3xx → OK/UNSET
				span.SetStatus(codes.Ok, "")
			case statusCode >= 400 && statusCode < 500:
				// 4xx → ERROR with client error message
				span.SetStatus(codes.Error, http.StatusText(statusCode))
			case statusCode >= 500:
				// 5xx → ERROR
				span.SetStatus(codes.Error, http.StatusText(statusCode))
			}
		}
	}

	return http.HandlerFunc(handlerFunc)
}

