package metrics429

import (
	"net/http"

	"github.com/shortlink-org/go-sdk/http/client/internal/types"
)

type Config struct {
	Metrics *types.Metrics
	Client  string
}

func Middleware(cfg Config) types.Middleware {
	if cfg.Metrics == nil {
		return func(next http.RoundTripper) http.RoundTripper { return next }
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			resp, err := next.RoundTrip(req)
			if err != nil {
				return nil, err
			}

			if resp.StatusCode == http.StatusTooManyRequests {
				cfg.Metrics.RateLimit429Total.
					WithLabelValues(cfg.Client, req.URL.Host, req.Method).
					Inc()
			}

			return resp, nil
		})
	}
}
