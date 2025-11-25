package handlers

import (
	wmmessage "github.com/ThreeDotsLabs/watermill/message"
)

// AsMiddleware converts decorator config into Watermill middleware for router-level usage.
func AsMiddleware(cfg DecoratorConfig) wmmessage.HandlerMiddleware {
	return func(next wmmessage.HandlerFunc) wmmessage.HandlerFunc {
		return DecorateHandler(next, cfg)
	}
}

// Chain applies handler middlewares sequentially.
func Chain(h wmmessage.HandlerFunc, middlewares ...wmmessage.HandlerMiddleware) wmmessage.HandlerFunc {
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		h = mw(h)
	}
	return h
}
