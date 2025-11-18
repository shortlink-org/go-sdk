package span_middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
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
	}

	return http.HandlerFunc(handlerFunc)
}
