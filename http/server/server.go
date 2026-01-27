package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/shortlink-org/go-sdk/config"
)

// New creates a new HTTP server with the given handler and configuration.
// It sets up timeouts and wraps the handler with a timeout handler.
func New(ctx context.Context, handler http.Handler, serverConfig Config, cfg *config.Config) *http.Server {
	// Set default timeouts
	cfg.SetDefault("HTTP_SERVER_READ_TIMEOUT", "5s")
	cfg.SetDefault("HTTP_SERVER_WRITE_TIMEOUT", "5s")
	cfg.SetDefault("HTTP_SERVER_IDLE_TIMEOUT", "30s")
	cfg.SetDefault("HTTP_SERVER_READ_HEADER_TIMEOUT", "2s")

	//nolint:gosec,exhaustruct // timeouts configured via viper immediately below
	server := &http.Server{}
	server.Addr = fmt.Sprintf(":%d", serverConfig.Port)
	server.Handler = http.TimeoutHandler(handler, serverConfig.Timeout, TimeoutMessage)
	server.BaseContext = func(_ net.Listener) context.Context { return ctx }
	server.ReadTimeout = cfg.GetDuration("HTTP_SERVER_READ_TIMEOUT")
	server.WriteTimeout = serverConfig.Timeout + cfg.GetDuration("HTTP_SERVER_WRITE_TIMEOUT")
	server.IdleTimeout = cfg.GetDuration("HTTP_SERVER_IDLE_TIMEOUT")
	server.ReadHeaderTimeout = cfg.GetDuration("HTTP_SERVER_READ_HEADER_TIMEOUT")

	return server
}
