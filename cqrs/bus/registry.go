package bus

import (
	"errors"
	"reflect"
	"sync"

	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
)

var (
	// ErrNilCommandType indicates that RegisterCommand received a nil value.
	ErrNilCommandType = errors.New("cqrs/bus: command type is nil")
	// ErrNilEventType indicates that RegisterEvent received a nil value.
	ErrNilEventType = errors.New("cqrs/bus: event type is nil")
)

// TypeRegistry stores command and event Go types mapped to canonical names.
type TypeRegistry struct {
	mu       sync.RWMutex
	commands map[string]reflect.Type
	events   map[string]reflect.Type
}

// NewTypeRegistry creates an empty registry.
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		commands: make(map[string]reflect.Type),
		events:   make(map[string]reflect.Type),
	}
}

// RegisterCommand registers command type.
func (r *TypeRegistry) RegisterCommand(cmd any) error {
	err := r.validateCommand(cmd)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name := cqrsmessage.NameOf(cmd)
	r.commands[name] = normalizeType(cmd)

	return nil
}

// RegisterEvent registers event type.
func (r *TypeRegistry) RegisterEvent(evt any) error {
	err := r.validateEvent(evt)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name := cqrsmessage.NameOf(evt)
	r.events[name] = normalizeType(evt)

	return nil
}

// ResolveCommand returns command type by canonical name.
func (r *TypeRegistry) ResolveCommand(name string) (reflect.Type, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmdType, ok := r.commands[name]

	return cmdType, ok
}

// ResolveEvent returns event type by canonical name.
func (r *TypeRegistry) ResolveEvent(name string) (reflect.Type, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	evtType, ok := r.events[name]

	return evtType, ok
}

// validateCommand checks that command is not nil.
func (r *TypeRegistry) validateCommand(cmd any) error {
	if cmd == nil {
		return ErrNilCommandType
	}

	return nil
}

// validateEvent checks that event is not nil.
func (r *TypeRegistry) validateEvent(evt any) error {
	if evt == nil {
		return ErrNilEventType
	}

	return nil
}

func normalizeType(v any) reflect.Type {
	typ := reflect.TypeOf(v)
	if typ == nil {
		return nil
	}

	if typ.Kind() != reflect.Pointer {
		typ = reflect.PointerTo(typ)
	}

	return typ
}
