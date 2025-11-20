package http_server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/shortlink-org/go-sdk/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

func New(ctx context.Context, h http.Handler, serverConfig Config, tracer trace.TracerProvider, cfg *config.Config) *http.Server {
	cfg.SetDefault("HTTP_SERVER_READ_TIMEOUT", "5s")        // the maximum duration for reading the entire request, including the body
	cfg.SetDefault("HTTP_SERVER_WRITE_TIMEOUT", "5s")       // the maximum duration before timing out writes of the response
	cfg.SetDefault("HTTP_SERVER_IDLE_TIMEOUT", "30s")       // the maximum amount of time to wait for the next request when keep-alive is enabled
	cfg.SetDefault("HTTP_SERVER_READ_HEADER_TIMEOUT", "2s") // the amount of time allowed to read request headers

	handler := http.TimeoutHandler(h, serverConfig.Timeout, fmt.Sprintf(`{"error": %q}`, TimeoutMessage))

	if tracer != nil {
		handler = otelhttp.NewHandler(handler, "")
	}

	server := &http.Server{} //nolint:gosec,exhaustruct // timeouts configured via viper immediately below
	server.Addr = fmt.Sprintf(":%d", serverConfig.Port)
	server.Handler = handler
	server.BaseContext = func(_ net.Listener) context.Context { return ctx }
	server.ReadTimeout = cfg.GetDuration("HTTP_SERVER_READ_TIMEOUT")
	server.WriteTimeout = serverConfig.Timeout + cfg.GetDuration("HTTP_SERVER_WRITE_TIMEOUT")
	server.IdleTimeout = cfg.GetDuration("HTTP_SERVER_IDLE_TIMEOUT")
	server.ReadHeaderTimeout = cfg.GetDuration("HTTP_SERVER_READ_HEADER_TIMEOUT")

	return server
}
