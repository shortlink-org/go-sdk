package handlers

import (
	"reflect"
	"testing"
)

type sampleStruct struct{}

type sampleInterface interface {
	Do()
}

type sampleImpl struct{}

func (sampleImpl) Do() {}

func TestNewValueHandlesPointerAndValueTypes(t *testing.T) {
	t.Run("value type returns pointer", func(t *testing.T) {
		typ := reflect.TypeOf(sampleStruct{})
		val := newValue(typ)
		if _, ok := val.(*sampleStruct); !ok {
			t.Fatalf("expected *sampleStruct, got %T", val)
		}
	})

	t.Run("pointer type is preserved", func(t *testing.T) {
		typ := reflect.TypeOf(&sampleStruct{})
		val := newValue(typ)
		if _, ok := val.(*sampleStruct); !ok {
			t.Fatalf("expected *sampleStruct, got %T", val)
		}
	})
}

func TestTypedPayloadCoversInterfacesAndValues(t *testing.T) {
	registryType := reflect.TypeOf(&sampleStruct{})
	handlerType := handlerTypeOf[*sampleStruct]()

	payload, err := typedPayload[*sampleStruct](&sampleStruct{}, handlerType, registryType)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload == nil {
		t.Fatal("payload is nil")
	}

	interfaceHandlerType := handlerTypeOf[sampleInterface]()
	_, err = typedPayload[sampleInterface](&sampleImpl{}, interfaceHandlerType, registryType)
	if err != nil {
		t.Fatalf("unexpected error for interface handler: %v", err)
	}
}

func TestTypedPayloadMismatch(t *testing.T) {
	registryType := reflect.TypeOf(&sampleStruct{})
	handlerType := handlerTypeOf[*sampleImpl]()

	if _, err := typedPayload[*sampleImpl](&sampleStruct{}, handlerType, registryType); err == nil {
		t.Fatal("expected handler type mismatch error")
	}
}
