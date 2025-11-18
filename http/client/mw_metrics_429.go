package http_client

import "net/http"

type Metrics429Config struct {
	Metrics *Metrics
	Client  string
}

func Metrics429Middleware(cfg Metrics429Config) Middleware {
	if cfg.Metrics == nil {
		return func(next http.RoundTripper) http.RoundTripper { return next }
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
