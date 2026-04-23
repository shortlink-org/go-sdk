package singleflightmiddleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/http/middleware/logger/mocks"
)

func TestSingleFlight_CoalescesRequests(t *testing.T) {
	t.Parallel()

	var handlerCalls atomic.Int32

	responseBody := "test response body"

	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		handlerCalls.Add(1)
		time.Sleep(50 * time.Millisecond) // Simulate slow handler
		writer.Header().Set("X-Custom-Header", "test-value")
		writer.WriteHeader(http.StatusOK)

		_, werr := writer.Write([]byte(responseBody))
		assert.NoError(t, werr)
	})

	mockLog := mocks.NewMockLogger(t)
	middleware := SingleFlight(mockLog)
	wrapped := middleware(handler)

	// Launch concurrent requests
	const numRequests = 10

	var waitGroup sync.WaitGroup

	responses := make([]*httptest.ResponseRecorder, numRequests)
	for idx := range numRequests {
		waitGroup.Add(1)

		responses[idx] = httptest.NewRecorder()

		go func(index int) {
			defer waitGroup.Done()

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test?foo=bar", http.NoBody)
			wrapped.ServeHTTP(responses[index], req)
		}(idx)
	}

	waitGroup.Wait()

	// Handler should only be called once
	assert.Equal(t, int32(1), handlerCalls.Load(), "handler should be called exactly once")

	// All responses should have the same body, status, and headers
	for idx, rec := range responses {
		assert.Equal(t, http.StatusOK, rec.Code, "response %d: unexpected status", idx)
		assert.Equal(t, responseBody, rec.Body.String(), "response %d: unexpected body", idx)
		assert.Equal(t, "test-value", rec.Header().Get("X-Custom-Header"), "response %d: missing header", idx)
	}
}

func TestSingleFlight_NonGETNotCoalesced(t *testing.T) {
	t.Parallel()

	var handlerCalls atomic.Int32

	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		handlerCalls.Add(1)
		writer.WriteHeader(http.StatusCreated)
	})

	mockLog := mocks.NewMockLogger(t)
	middleware := SingleFlight(mockLog)
	wrapped := middleware(handler)

	// Launch concurrent POST requests
	const numRequests = 5

	var waitGroup sync.WaitGroup

	for range numRequests {
		waitGroup.Go(func() {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/test", http.NoBody)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)
		})
	}

	waitGroup.Wait()

	// Each POST should call handler
	assert.Equal(t, int32(numRequests), handlerCalls.Load(), "POST requests should not be coalesced")
}

func TestSingleFlight_DifferentKeysNotCoalesced(t *testing.T) {
	t.Parallel()

	var handlerCalls atomic.Int32

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		handlerCalls.Add(1)
		time.Sleep(30 * time.Millisecond)

		//nolint:gosec // test handler echoes path into body for assertions
		_, werr := writer.Write([]byte(request.URL.Path))
		assert.NoError(t, werr)
	})

	mockLog := mocks.NewMockLogger(t)
	middleware := SingleFlight(mockLog)
	wrapped := middleware(handler)

	var waitGroup sync.WaitGroup

	paths := []string{"/path1", "/path2", "/path3"}
	for _, path := range paths {
		waitGroup.Add(1)

		go func(urlPath string) {
			defer waitGroup.Done()

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, urlPath, http.NoBody)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)
		}(path)
	}

	waitGroup.Wait()

	// Each unique path should call handler
	//nolint:gosec // len(paths) is always small
	assert.Equal(t, int32(len(paths)), handlerCalls.Load(), "different keys should not be coalesced")
}

func TestSingleFlight_PreservesStatusCode(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)

		_, werr := writer.Write([]byte("not found"))
		assert.NoError(t, werr)
	})

	mockLog := mocks.NewMockLogger(t)
	middleware := SingleFlight(mockLog)
	wrapped := middleware(handler)

	var waitGroup sync.WaitGroup

	const numRequests = 3

	recorders := make([]*httptest.ResponseRecorder, numRequests)
	for idx := range numRequests {
		waitGroup.Add(1)

		recorders[idx] = httptest.NewRecorder()

		go func(index int) {
			defer waitGroup.Done()

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/notfound", http.NoBody)
			wrapped.ServeHTTP(recorders[index], req)
		}(idx)
	}

	waitGroup.Wait()

	for idx, rec := range recorders {
		assert.Equal(t, http.StatusNotFound, rec.Code, "response %d: status not preserved", idx)
		assert.Equal(t, "not found", rec.Body.String(), "response %d: body not preserved", idx)
	}
}

func TestSingleFlight_Integration(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)

		_, werr := writer.Write([]byte(`{"status":"ok"}`))
		assert.NoError(t, werr)
	})

	mockLog := mocks.NewMockLogger(t)
	middleware := SingleFlight(mockLog)

	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/test", http.NoBody)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.JSONEq(t, `{"status":"ok"}`, string(body))
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}
