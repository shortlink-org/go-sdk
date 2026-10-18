package span_middleware

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/http/internal/semconvhttp"
)

const (
	// TraceIDHeader is the header key for the trace id, in the canonical form
	// net/http uses on the wire.
	TraceIDHeader = "Trace-Id"
)

type span struct{}

// Span is a middleware that completes the server span otelhttp opened and
// echoes its trace id to the client.
//
// The span is created upstream, by httpserver.New when it was given a tracer
// provider, and otelhttp records what it can see from outside the router:
// method, scheme, path, status. The matched route is resolved further in,
// where that wrapper no longer sees it, so this middleware runs inside the
// router and adds what the HTTP semantic conventions expect from there:
//
//   - the span is renamed "{method} {http.route}", and http.route is set on
//     the span and on the otelhttp metrics for the request;
//   - a 5xx response marks the span as failed and sets error.type to the
//     status code, which the conventions require on every failed span;
//   - a 4xx response leaves the status unset, because a client error is not
//     a server failure, and the status is never set to OK, which is reserved
//     for application code.
//
// The route comes from chi's route context when the request went through a
// chi router, and from the request pattern net/http's ServeMux records
// otherwise. Without either the span keeps its method-only name.
func Span() func(next http.Handler) http.Handler {
	return span{}.middleware
}

func (s span) middleware(next http.Handler) http.Handler {
	handlerFunc := func(writer http.ResponseWriter, request *http.Request) {
		wrappedWriter := middleware.NewWrapResponseWriter(writer, request.ProtoMajor)

		// Get span from context once. The span is created upstream — by
		// httpserver.New when it was given a tracer provider — and this
		// middleware only reads it, never starts one of its own.
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

		if !span.IsRecording() {
			return
		}

		// The route is known only after the router matched, which is why
		// this runs after next and not before.
		if route := routeOf(request); route != "" {
			span.SetName(semconvhttp.SpanMethod(request.Method) + " " + route)
			span.SetAttributes(semconv.HTTPRoute(route))

			// otelhttp never sees the match either, so its metrics would be
			// recorded without a route. The labeler is the channel it offers
			// for attributes the handler learns later.
			if labeler, ok := otelhttp.LabelerFromContext(request.Context()); ok {
				labeler.Add(semconv.HTTPRoute(route))
			}
		}

		// Only a 5xx is a server failure. Instrumentation leaves everything
		// else UNSET: OK is for application code that verified the outcome,
		// and a 4xx is the client's error, not the server's.
		if statusCode := wrappedWriter.Status(); statusCode >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, "")
			span.SetAttributes(semconv.ErrorTypeKey.String(strconv.Itoa(statusCode)))
		}
	}

	return http.HandlerFunc(handlerFunc)
}

// routeOf returns the route template the router matched for the request, or
// the empty string when nothing records one.
func routeOf(request *http.Request) string {
	// chi keeps the pattern in its own context and finishes it only once the
	// handler returned. RouteContext is nil outside a chi router, and
	// RoutePattern is nil-safe.
	if route := chi.RouteContext(request.Context()).RoutePattern(); route != "" {
		return route
	}

	// net/http's ServeMux writes the pattern it matched onto the request
	// itself, so a middleware wrapping the mux reads it after the fact.
	return semconvhttp.Route(request.Pattern)
}
