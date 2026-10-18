// Package semconvhttp holds the parts of the HTTP semantic conventions that
// otelhttp leaves to the application and that both sides of this module need:
// the method token a span name starts with, and the route a server span is
// named after.
//
// https://opentelemetry.io/docs/specs/semconv/http/http-spans/
package semconvhttp

import (
	"net/http"
	"strings"
)

// otherMethod is the token the conventions give a method the instrumentation
// does not recognize, so that a made-up method cannot turn each request into
// a new span name.
const otherMethod = "HTTP"

// knownMethods are the methods the conventions list for http.request.method.
// Any other value is reported as "_OTHER" in the attribute and as "HTTP" in
// the span name.
//
//nolint:gochecknoglobals // a fixed lookup table, never written after init
var knownMethods = map[string]struct{}{
	http.MethodConnect: {},
	http.MethodDelete:  {},
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodOptions: {},
	http.MethodPatch:   {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodTrace:   {},
}

// SpanMethod returns the method token a span name starts with: the method
// itself when the conventions know it, "HTTP" otherwise. Matching is
// case-sensitive, as the conventions require; "get" is not GET.
func SpanMethod(method string) string {
	if _, ok := knownMethods[method]; ok {
		return method
	}

	return otherMethod
}

// Route extracts the http.route value from a router pattern.
//
// The ServeMux of net/http reports "[METHOD ][HOST]/path", and the route is
// the path template alone, so everything before the first slash is dropped. A
// chi pattern has no prefix and is returned unchanged. A pattern without a
// slash is not a route at all, and the empty string says so.
func Route(pattern string) string {
	if idx := strings.IndexByte(pattern, '/'); idx >= 0 {
		return pattern[idx:]
	}

	return ""
}
