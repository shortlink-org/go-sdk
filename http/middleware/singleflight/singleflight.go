package singleflight_middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/shortlink-org/go-sdk/logger"
	"golang.org/x/sync/singleflight"
)

type Option func(*singleFlight)

func WithKeyFn(keyFn func(r *http.Request) string) Option {
	return func(s *singleFlight) {
		s.keyFn = keyFn
	}
}

type singleFlight struct {
	log   logger.Logger
	group singleflight.Group
	keyFn func(r *http.Request) string
}

// SingleFlight is a middleware that prevents duplicate requests from hitting your server.
func SingleFlight(log logger.Logger, options ...Option) func(next http.Handler) http.Handler {
	// Default keyFn is path + query
	keyFn := func(r *http.Request) string {
		return fmt.Sprintf("%s?%s", r.URL.Path, r.URL.RawQuery)
	}

	singleFlightInstance := new(singleFlight)
	singleFlightInstance.log = log
	singleFlightInstance.keyFn = keyFn

	for _, option := range options {
		option(singleFlightInstance)
	}

	return singleFlightInstance.middleware
}

func (s *singleFlight) middleware(next http.Handler) http.Handler {
	handlerFunc := func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			key := s.keyFn(request)

			response, err, _ := s.group.Do(key, func() (any, error) {
				next.ServeHTTP(writer, request)

				//nolint:nilnil // nil, nil is valid return
				return nil, nil
			})
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)

				return
			}

			_, err = fmt.Fprint(writer, response)
			if err != nil {
				s.log.Error("failed to write response",
					slog.Any("error", err),
				)
			}
		} else {
			next.ServeHTTP(writer, request)
		}
	}

	return http.HandlerFunc(handlerFunc)
}
