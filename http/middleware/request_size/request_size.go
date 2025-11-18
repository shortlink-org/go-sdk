package request_size_middleware

import (
	"net/http"
)

// RequestSize is a middleware that will limit request sizes to a specified
// number of bytes. It uses MaxBytesReader to do so.
//
// Size:
// 1<<10 - 1KB
// 1<<20 - 1MB
// 1<<30 - 1GB
func RequestSize(bytes int64) func(http.Handler) http.Handler {
	wrapRequestSize := func(handler http.Handler) http.Handler {
		handlerFunc := func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, bytes)
			handler.ServeHTTP(w, r)
		}

		return http.HandlerFunc(handlerFunc)
	}

	return wrapRequestSize
}
