package http_client

import "net/http"

type TokenBucketConfig struct {
	Limiter Limiter
	Metrics *Metrics
	Client  string
}

func TokenBucketMiddleware(cfg TokenBucketConfig) Middleware {
	if cfg.Limiter == nil {
		return func(next http.RoundTripper) http.RoundTripper { return next }
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			wait, err := cfg.Limiter.Wait(req.Context())
			if err != nil {
				return nil, err
			}

			recordWait(req.Context(), "bucket", wait)

			if wait > 0 && cfg.Metrics != nil {
				cfg.Metrics.RateLimitWaitSeconds.
					WithLabelValues(cfg.Client, req.URL.Host, req.Method, "bucket").
					Observe(wait.Seconds())
			}

			return next.RoundTrip(req)
		})
	}
}
