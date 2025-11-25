package handlers

import "errors"

var (
	errNilMessage           = errors.New("cqrs/handlers: message is nil")
	errNilCommandLogic      = errors.New("cqrs/handlers: command handler logic is nil")
	errNilEventLogic        = errors.New("cqrs/handlers: event handler logic is nil")
	errNilRegistry          = errors.New("cqrs/handlers: type registry is nil")
	errNilMarshaler         = errors.New("cqrs/handlers: marshaler is nil")
	errCommandNotRegistered = errors.New("cqrs/handlers: command is not registered")
	errEventNotRegistered   = errors.New("cqrs/handlers: event is not registered")
	errHandlerTypeMismatch  = errors.New("cqrs/handlers: handler type mismatch")
)
