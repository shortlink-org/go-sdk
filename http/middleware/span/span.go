package span_middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TraceIDHeader is the header key for the trace id.
	TraceIDHeader = "trace_id"
)

type span struct{}

// Span is a middleware that adds a span to the response context.
func Span() func(next http.Handler) http.Handler {
	return span{}.middleware
}

func (s span) middleware(next http.Handler) http.Handler {
	handlerFunc := func(writer http.ResponseWriter, request *http.Request) {
		wrappedWriter := middleware.NewWrapResponseWriter(writer, request.ProtoMajor)

		// Check if "trace_id" already exists in the header
		if wrappedWriter.Header().Get(TraceIDHeader) == "" {
			// Inject spanId in response header
			wrappedWriter.Header().Add(TraceIDHeader, trace.SpanFromContext(request.Context()).SpanContext().TraceID().String())
		}

		next.ServeHTTP(wrappedWriter, request)

		// Set OTEL span status based on HTTP status code
		span := trace.SpanFromContext(request.Context())
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
