package tokenbucket

import (
	"net/http"

	"github.com/shortlink-org/go-sdk/http/client/internal/types"
	"github.com/shortlink-org/go-sdk/http/client/middleware/otelwait"
)

type Config struct {
	Limiter types.Limiter
	Metrics *types.Metrics
	Client  string
}

func Middleware(cfg Config) types.Middleware {
	if cfg.Limiter == nil {
		return func(next http.RoundTripper) http.RoundTripper { return next }
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			wait, err := cfg.Limiter.Wait(req.Context())
			if err != nil {
				return nil, err
			}

			otelwait.RecordWait(req.Context(), "bucket", wait)

			if wait > 0 && cfg.Metrics != nil {
				cfg.Metrics.RateLimitWaitSeconds.
					WithLabelValues(cfg.Client, req.URL.Host, req.Method, "bucket").
					Observe(wait.Seconds())
			}

			return next.RoundTrip(req)
		})
	}
}
