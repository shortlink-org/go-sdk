package saga

import (
	"log/slog"
)

type Options struct {
	log     *slog.Logger
	limiter int
}

type Option func(*Options)

func SetLogger(log *slog.Logger) Option {
	return func(args *Options) {
		args.log = log
	}
}

func SetLimiter(limiter int) Option {
	return func(args *Options) {
		args.limiter = limiter
	}
}
