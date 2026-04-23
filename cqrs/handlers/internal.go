package handlers

import (
	"fmt"
	"reflect"
)

func newValue(typ reflect.Type) any {
	if typ == nil {
		return nil
	}

	// Ensure we always produce a pointer value so proto.Unmarshal works.
	if typ.Kind() != reflect.Pointer {
		typ = reflect.PointerTo(typ)
	}

	return reflect.New(typ.Elem()).Interface()
}

func handlerTypeOf[T any]() reflect.Type {
	var zero T
	return reflect.TypeOf(zero)
}

//
//nolint:ireturn // Returns the concrete generic payload type T.
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
	default:
		// mutually assignable concrete types
	}

	payload, ok := instance.(T)
	if !ok {
		return zero, fmt.Errorf("%w: registry=%s handler=%s", errHandlerTypeMismatch, registryType, handlerType)
	}

	return payload, nil
}
