package httpserver

// Error response messages (pre-formatted JSON).
const (
	// TimeoutMessage is the response body for request timeouts.
	TimeoutMessage = `{"error":"context deadline exceeded"}`
	// CSRFMessage is the response body for CSRF protection blocks.
	CSRFMessage = `{"error":"cross-origin request blocked"}`
)
