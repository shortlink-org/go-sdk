package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
)

// New creates a new HTTP server with the given handler and configuration.
// It sets up timeouts and wraps the handler with a timeout handler, and with
// otelhttp when WithTracer supplies a provider.
func New(
	ctx context.Context,
	handler http.Handler,
	serverConfig Config,
	cfg *config.Config,
	opts ...Option,
) *http.Server {
	serverSettings := &settings{}
	for _, opt := range opts {
		opt(serverSettings)
	}

	// Set default timeouts
	cfg.SetDefault("HTTP_SERVER_READ_TIMEOUT", "5s")
	cfg.SetDefault("HTTP_SERVER_WRITE_TIMEOUT", "5s")
	cfg.SetDefault("HTTP_SERVER_IDLE_TIMEOUT", "30s")
	cfg.SetDefault("HTTP_SERVER_READ_HEADER_TIMEOUT", "2s")

	// A single header repeated thousands of times costs the server memory and
	// CPU long before any timeout fires, so cap the count as well as the size.
	// http.DefaultMaxHeaderValueCount is what net/http applies on its own when
	// the field is zero; naming it here makes the limit visible and tunable.
	cfg.SetDefault("HTTP_SERVER_MAX_HEADER_VALUE_COUNT", http.DefaultMaxHeaderValueCount)

	//nolint:gosec,exhaustruct // timeouts configured via viper immediately below
	server := &http.Server{}
	server.Addr = fmt.Sprintf(":%d", serverConfig.Port)
	server.Handler = withTracing(
		http.TimeoutHandler(handler, serverConfig.Timeout, TimeoutMessage),
		serverSettings.tracer,
	)
	server.BaseContext = func(_ net.Listener) context.Context { return ctx }
	server.ReadTimeout = cfg.GetDuration("HTTP_SERVER_READ_TIMEOUT")
	server.WriteTimeout = serverConfig.Timeout + cfg.GetDuration("HTTP_SERVER_WRITE_TIMEOUT")
	server.IdleTimeout = cfg.GetDuration("HTTP_SERVER_IDLE_TIMEOUT")
	server.ReadHeaderTimeout = cfg.GetDuration("HTTP_SERVER_READ_HEADER_TIMEOUT")
	server.MaxHeaderValueCount = cfg.GetInt("HTTP_SERVER_MAX_HEADER_VALUE_COUNT")

	return server
}

// withTracing puts the otel handler outside the timeout handler, so that a
// request that runs out of time is still reported as one span, carrying the 503
// the timeout handler wrote. Span names follow the otelhttp default, which is
// the request method: the route pattern is resolved by the router further in,
// where this wrapper can no longer see it.
//
//nolint:ireturn // http.Handler is the type the caller asked for
func withTracing(handler http.Handler, tracer trace.TracerProvider) http.Handler {
	if tracer == nil {
		return handler
	}

	return otelhttp.NewHandler(handler, "", otelhttp.WithTracerProvider(tracer))
}
