package deadline

import (
	"net/http"
	"time"

	"github.com/shortlink-org/go-sdk/http/client/internal/types"
)

type Config struct {
	Threshold time.Duration
	Metrics   *types.Metrics
	Client    string
}

func Middleware(cfg Config) types.Middleware {
	if cfg.Threshold <= 0 {
		return func(next http.RoundTripper) http.RoundTripper { return next }
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			ctx := req.Context()

			dl, ok := ctx.Deadline()
			// Check if deadline is too close, accounting for potential clock skew.
			// Using time.Until ensures we check relative to current time.
			if ok && time.Until(dl) < cfg.Threshold {
				if cfg.Metrics != nil {
					cfg.Metrics.DeadlineCancelledTotal.
						WithLabelValues(cfg.Client, req.URL.Host, req.Method).
						Inc()
				}

				return nil, types.ErrDeadlineTooClose
			}

			return next.RoundTrip(req)
		})
	}
}
