package router

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ThreeDotsLabs/watermill"
	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	wmmid "github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/shortlink-org/go-sdk/cqrs/handlers"
)

var (
	errNilLogger       = errors.New("cqrs/router: watermill logger is required")
	errNilSubscriber   = errors.New("cqrs/router: subscriber is required")
	errNilPublisher    = errors.New("cqrs/router: publisher is required")
	errNoHandlers      = errors.New("cqrs/router: at least one handler must be configured")
	errNilHandlerLogic = errors.New("cqrs/router: handler function is nil")
)

// NewRouter builds CQRS-aware Watermill router.
func NewRouter(
	logger watermill.LoggerAdapter,
	subscriber wmmessage.Subscriber,
	publisher wmmessage.Publisher,
	cfg RouterConfig,
) (*wmmessage.Router, error) {
	if logger == nil {
		return nil, errNilLogger
	}
	if subscriber == nil {
		return nil, errNilSubscriber
	}
	if publisher == nil {
		return nil, errNilPublisher
	}
	if len(cfg.Handlers) == 0 {
		return nil, errNoHandlers
	}

	router, err := wmmessage.NewRouter(wmmessage.RouterConfig{}, logger)
	if err != nil {
		return nil, err
	}

	applyBaseMiddlewares(router)

	decoratorCfg := handlers.DecoratorConfig{
		Timeout:                cfg.Middlewares.Timeout,
		RetryMax:               cfg.Middlewares.RetryMax,
		CircuitBreakerEnabled:  cfg.Middlewares.CircuitBreakerEnabled,
		CircuitBreakerSettings: cfg.Middlewares.CircuitBreakerSettings,
	}

	service := sanitizeService(cfg.ServiceName)
	for _, registration := range enumerateHandlers(cfg, service) {
		if registration.Handler == nil {
			return nil, fmt.Errorf("%w: topic %s", errNilHandlerLogic, registration.Topic)
		}
		if registration.Topic == "" {
			return nil, fmt.Errorf("cqrs/router: topic is empty for handler %s", registration.Name)
		}

		decorated := handlers.DecorateHandler(registration.Handler, decoratorCfg)
		router.AddHandler(registration.Name, registration.Topic, subscriber, "", publisher, decorated)
	}

	return router, nil
}

func applyBaseMiddlewares(router *wmmessage.Router) {
	router.AddMiddleware(wmmid.Recoverer)
	router.AddMiddleware(wmmid.CorrelationID)
}

func sanitizeService(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "cqrs"
	}
	return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
}

func sanitizeTopic(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "topic"
	}
	return strings.ToLower(strings.ReplaceAll(name, "*", "wildcard"))
}

func enumerateHandlers(cfg RouterConfig, service string) []HandlerRegistration {
	regs := make([]HandlerRegistration, 0, len(cfg.Handlers))
	for _, h := range cfg.Handlers {
		regs = append(regs, h.sanitize(service))
	}
	return regs
}
