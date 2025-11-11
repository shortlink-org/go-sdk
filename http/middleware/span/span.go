package span_middleware

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TraceIDHeader is the header key for the trace id.
	TraceIDHeader = "trace_id"
)

type span struct{}

// Span injects the active span's trace id into the response headers and adds
// route metadata (http.route + normalized span name) to improve observability.
func Span() func(next http.Handler) http.Handler {
	return span{}.middleware
}

func (s span) middleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		currentSpan := trace.SpanFromContext(r.Context())

		// Check if "trace_id" already exists in the header.
		if ww.Header().Get(TraceIDHeader) == "" {
			spanCtx := currentSpan.SpanContext()
			if spanCtx.HasTraceID() {
				// Inject traceId in response header.
				ww.Header().Add(TraceIDHeader, spanCtx.TraceID().String())
			}
		}

		next.ServeHTTP(ww, r)

		if currentSpan.IsRecording() {
			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				routePattern := strings.Join(rctx.RoutePatterns, "")
				routePattern = strings.ReplaceAll(routePattern, "/*/", "/")

				if routePattern != "" {
					currentSpan.SetAttributes(attribute.String("http.route", routePattern))
					currentSpan.SetName(r.Method + " " + routePattern)
				}
			}
		}
	}

	return http.HandlerFunc(fn)
}
