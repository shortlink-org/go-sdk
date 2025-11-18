package http_client

import (
	"context"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type OTelWaitConfig struct {
	Client string
}

func OTelWaitMiddleware(cfg OTelWaitConfig) Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			ctx := req.Context()
			tr := otel.Tracer("http_client")

			ctx, span := tr.Start(ctx, "rate_limit_wait")
			defer span.End()

			tracker := newWaitTracker(span)
			ctx = context.WithValue(ctx, waitTrackerKey, tracker)

			resp, err := next.RoundTrip(req.WithContext(ctx))

			totalWait := tracker.Total()
			span.SetAttributes(
				attribute.String("client", cfg.Client),
				attribute.String("host", req.URL.Host),
				attribute.String("method", req.Method),
				attribute.Int64("wait_total_ms", totalWait.Milliseconds()),
			)

			return resp, err
		})
	}
}

type waitTracker struct {
	mu    sync.Mutex
	span  trace.Span
	total time.Duration
}

func newWaitTracker(span trace.Span) *waitTracker {
	tracker := new(waitTracker)
	tracker.span = span

	return tracker
}

func (t *waitTracker) Total() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.total
}

func (t *waitTracker) add(source string, duration time.Duration) {
	if duration <= 0 {
		return
	}

	t.mu.Lock()
	t.total += duration
	t.mu.Unlock()

	t.span.AddEvent("wait",
		trace.WithAttributes(
			attribute.String("source", source),
			attribute.Int64("duration_ms", duration.Milliseconds()),
		))
}

type waitTrackerContextKey struct{}

var waitTrackerKey = waitTrackerContextKey{}

func getWaitTracker(ctx context.Context) *waitTracker {
	value := ctx.Value(waitTrackerKey)

	tracker, ok := value.(*waitTracker)
	if !ok {
		return nil
	}

	return tracker
}

func recordWait(ctx context.Context, source string, d time.Duration) {
	if tracker := getWaitTracker(ctx); tracker != nil {
		tracker.add(source, d)
	}
}
