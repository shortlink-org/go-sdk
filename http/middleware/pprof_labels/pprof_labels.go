package pprof_labels_middleware

import (
	"net/http"
	"runtime/pprof"
)

// Labels is a middleware function that adds pprof labels to the context of the incoming HTTP request.
// These labels include the request path and method.
// The updated context is then used to serve the HTTP request.
func Labels(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx := pprof.WithLabels(request.Context(), pprof.Labels(
			"path", request.URL.Path,
			"method", request.Method,
		))

		pprof.SetGoroutineLabels(ctx)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}
