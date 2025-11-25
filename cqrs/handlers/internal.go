package handlers

import (
	"fmt"
	"reflect"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
)

func newValue(t reflect.Type) any {
	if t == nil {
		return nil
	}

	// Ensure we always produce a pointer value so proto.Unmarshal works.
	if t.Kind() != reflect.Pointer {
		t = reflect.PointerTo(t)
	}

	return reflect.New(t.Elem()).Interface()
}

func cloneMetadata(meta wmmessage.Metadata) map[string]string {
	if meta == nil {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func handlerTypeOf[T any]() reflect.Type {
	var zero T
	return reflect.TypeOf(zero)
}

func typedPayload[T any](instance any, handlerType, registryType reflect.Type) (T, error) {
	var zero T

	if instance == nil {
		return zero, fmt.Errorf("%w: nil instance", errHandlerTypeMismatch)
	}

	instanceType := reflect.TypeOf(instance)

	if handlerType == nil {
		payload, ok := instance.(T)
		if !ok {
			return zero, fmt.Errorf("%w: registry=%v handler=%T", errHandlerTypeMismatch, registryType, zero)
		}
		return payload, nil
	}

	if registryType == nil {
		return zero, fmt.Errorf("%w: registry type unknown for handler %s", errHandlerTypeMismatch, handlerType)
	}

	switch {
	case handlerType.Kind() == reflect.Interface:
		if !instanceType.Implements(handlerType) {
			return zero, fmt.Errorf("%w: registry=%s handler=%s", errHandlerTypeMismatch, registryType, handlerType)
		}
	case registryType.Kind() == reflect.Interface:
		if !handlerType.Implements(registryType) {
			return zero, fmt.Errorf("%w: registry=%s handler=%s", errHandlerTypeMismatch, registryType, handlerType)
		}
	case !registryType.AssignableTo(handlerType) && !handlerType.AssignableTo(registryType):
		return zero, fmt.Errorf("%w: registry=%s handler=%s", errHandlerTypeMismatch, registryType, handlerType)
	}

	payload, ok := instance.(T)
	if !ok {
		return zero, fmt.Errorf("%w: registry=%s handler=%s", errHandlerTypeMismatch, registryType, handlerType)
	}
	return payload, nil
}
