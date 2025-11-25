package router

import (
	"strings"
	"time"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"github.com/sony/gobreaker"
)

// RouterConfig describes CQRS router runtime parameters.
type RouterConfig struct {
	ServiceName string
	Handlers    []HandlerRegistration
	Middlewares RouterMiddlewareConfig
}

// HandlerRegistration wires a Watermill handler to a topic.
type HandlerRegistration struct {
	Name    string
	Topic   string
	Handler wmmessage.HandlerFunc
}

// RouterMiddlewareConfig configures CQRS decorator behavior.
type RouterMiddlewareConfig struct {
	Timeout                time.Duration
	RetryMax               int
	CircuitBreakerEnabled  bool
	CircuitBreakerSettings *gobreaker.Settings
}

func (h HandlerRegistration) sanitize(service string) HandlerRegistration {
	if h.Name == "" {
		h.Name = strings.Join([]string{service, sanitizeTopic(h.Topic), "handler"}, "_")
	}
	h.Topic = strings.TrimSpace(h.Topic)
	return h
}
