package httpserver

import (
	"go.opentelemetry.io/otel/trace"
)

// Option configures the server built by New.
type Option func(*settings)

// settings holds what the options collected, before New turns them into a server.
type settings struct {
	tracer trace.TracerProvider
}

// WithTracer wraps the handler in otelhttp, so that every request opens a server
// span and the incoming trace context is picked up from the request headers.
//
//	server := httpserver.New(ctx, router, config, cfg, httpserver.WithTracer(tracer))
//
// Without it the server stays uninstrumented: the wrapper is not free, and a
// service that does not export traces should not pay for it. A nil provider is
// the same as not passing the option at all, so wiring straight from a tracing
// setup that returned nothing is safe.
func WithTracer(tracer trace.TracerProvider) Option {
	return func(s *settings) {
		s.tracer = tracer
	}
}
