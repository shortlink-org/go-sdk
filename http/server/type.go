// Package httpserver provides HTTP server configuration and initialization.
package httpserver

import (
	"time"
)

// Config contains base configuration for the HTTP API server.
type Config struct {
	Port    int
	Timeout time.Duration
}
