package http_client

import (
	"net/http"
	"time"
)

type DeadlineConfig struct {
	Threshold time.Duration
	Metrics   *Metrics
	Client    string
}

func DeadlineMiddleware(cfg DeadlineConfig) Middleware {
	if cfg.Threshold <= 0 {
		return func(next http.RoundTripper) http.RoundTripper { return next }
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			ctx := req.Context()

			dl, ok := ctx.Deadline()
			if ok && time.Until(dl) < cfg.Threshold {
				if cfg.Metrics != nil {
					cfg.Metrics.DeadlineCancelledTotal.
						WithLabelValues(cfg.Client, req.URL.Host, req.Method).
						Inc()
				}

				return nil, ErrDeadlineTooClose
			}

			return next.RoundTrip(req)
		})
	}
}
