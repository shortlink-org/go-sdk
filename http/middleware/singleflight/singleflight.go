// Package singleflightmiddleware provides HTTP middleware that coalesces
// duplicate GET requests, ensuring only one request proceeds while others wait.
package singleflightmiddleware

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/shortlink-org/go-sdk/logger"
	"golang.org/x/sync/singleflight"
)

// Option configures the singleflight middleware.
type Option func(*singleFlight)

// WithKeyFn sets a custom function to generate cache keys from requests.
func WithKeyFn(keyFn func(r *http.Request) string) Option {
	return func(s *singleFlight) {
		s.keyFn = keyFn
	}
}

type singleFlight struct {
	log   logger.Logger
	group singleflight.Group
	keyFn func(r *http.Request) string
}

// bufferedResponse captures the response from the leader request.
type bufferedResponse struct {
	statusCode int
	header     http.Header
	body       []byte
}

// bufferedResponseWriter captures response data for replay to waiting requests.
type bufferedResponseWriter struct {
	buffer     *bytes.Buffer
	header     http.Header
	statusCode int
	mu         sync.Mutex
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{
		buffer:     new(bytes.Buffer),
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.header
}

func (w *bufferedResponseWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buffer.Write(data)
}

func (w *bufferedResponseWriter) WriteHeader(statusCode int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.statusCode = statusCode
}

func (w *bufferedResponseWriter) toBufferedResponse() *bufferedResponse {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Copy headers to avoid race conditions
	headerCopy := make(http.Header, len(w.header))
	for key, values := range w.header {
		headerCopy[key] = append([]string(nil), values...)
	}

	return &bufferedResponse{
		statusCode: w.statusCode,
		header:     headerCopy,
		body:       w.buffer.Bytes(),
	}
}

// replayTo writes the buffered response to the given ResponseWriter.
func (r *bufferedResponse) replayTo(writer http.ResponseWriter) error {
	// Copy headers
	for key, values := range r.header {
		for _, value := range values {
			writer.Header().Add(key, value)
		}
	}

	// Write status code
	writer.WriteHeader(r.statusCode)

	// Write body
	_, err := writer.Write(r.body)

	return err
}

// SingleFlight is a middleware that coalesces duplicate GET requests.
// Only one request proceeds to the handler; others wait and receive the same response.
func SingleFlight(log logger.Logger, options ...Option) func(next http.Handler) http.Handler {
	keyFn := func(r *http.Request) string {
		return fmt.Sprintf("%s?%s", r.URL.Path, r.URL.RawQuery)
	}

	singleFlightInstance := &singleFlight{
		log:   log,
		keyFn: keyFn,
	}

	for _, option := range options {
		option(singleFlightInstance)
	}

	return singleFlightInstance.middleware
}

func (s *singleFlight) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Only coalesce GET requests
		if request.Method != http.MethodGet {
			next.ServeHTTP(writer, request)

			return
		}

		key := s.keyFn(request)

		// Execute with singleflight - leader captures response, waiters receive it
		response, err, shared := s.group.Do(key, func() (any, error) {
			// Leader: capture response in buffer
			bufferedWriter := newBufferedResponseWriter()
			next.ServeHTTP(bufferedWriter, request)

			return bufferedWriter.toBufferedResponse(), nil
		})
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)

			return
		}

		// Type assert the response
		bufferedResp, ok := response.(*bufferedResponse)
		if !ok {
			s.log.Error("singleflight: unexpected response type")
			http.Error(writer, "internal error", http.StatusInternalServerError)

			return
		}

		// Replay buffered response to this request's writer
		replayErr := bufferedResp.replayTo(writer)
		if replayErr != nil {
			s.log.Error("failed to replay response",
				slog.Any("error", replayErr),
				slog.Bool("shared", shared),
			)
		}
	})
}
