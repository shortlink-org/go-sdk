package message

import (
	"strings"
	"testing"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
)

type testCommand struct {
	OrderId string  `json:"order_id"`
	Amount  float64 `json:"amount"`
}

type testEvent struct {
	OrderId   string `json:"order_id"`
	CreatedAt int64  `json:"created_at"`
}

func TestJSONMarshalerMarshal(t *testing.T) {
	namer := NewShortlinkNamer("test")
	m := NewJSONMarshaler(namer)

	cmd := &testCommand{
		OrderId: "order-123",
		Amount:  99.99,
	}

	msg, err := m.Marshal(cmd)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if len(msg.Payload) == 0 {
		t.Fatal("payload is empty")
	}

	if msg.Metadata.Get(MetadataContentType) != "application/json" {
		t.Errorf("expected content type 'application/json', got %s", msg.Metadata.Get(MetadataContentType))
	}

	if msg.Metadata.Get(MetadataServiceName) != "test" {
		t.Errorf("expected service name 'test', got %s", msg.Metadata.Get(MetadataServiceName))
	}

	if msg.Metadata.Get(MetadataMessageKind) != string(KindCommand) {
		t.Errorf("expected message kind 'command', got %s", msg.Metadata.Get(MetadataMessageKind))
	}
}

func TestJSONMarshalerUnmarshal(t *testing.T) {
	namer := NewShortlinkNamer("test")
	m := NewJSONMarshaler(namer)

	original := &testCommand{
		OrderId: "order-123",
		Amount:  99.99,
	}

	msg, err := m.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var unmarshaled testCommand
	if err := m.Unmarshal(msg, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if unmarshaled.OrderId != original.OrderId {
		t.Errorf("expected OrderId %s, got %s", original.OrderId, unmarshaled.OrderId)
	}

	if unmarshaled.Amount != original.Amount {
		t.Errorf("expected Amount %f, got %f", original.Amount, unmarshaled.Amount)
	}
}

func TestJSONMarshalerUnmarshalEmptyPayload(t *testing.T) {
	m := NewJSONMarshaler(NewShortlinkNamer("svc"))
	msg := wmmessage.NewMessage("id", nil)

	var cmd testCommand
	if err := m.Unmarshal(msg, &cmd); err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestJSONMarshalerUnmarshalNilMessage(t *testing.T) {
	m := NewJSONMarshaler(NewShortlinkNamer("svc"))

	var cmd testCommand
	if err := m.Unmarshal(nil, &cmd); err == nil {
		t.Fatal("expected error for nil message")
	}
}

func TestJSONMarshalerName(t *testing.T) {
	namer := NewShortlinkNamer("test")
	m := NewJSONMarshaler(namer)

	cmd := &testCommand{OrderId: "123"}
	name := m.Name(cmd)

	if name == "" {
		t.Fatal("name should not be empty")
	}

	if !strings.Contains(name, "test") {
		t.Errorf("name should contain 'test', got %s", name)
	}

	if !strings.Contains(name, "command") {
		t.Errorf("name should contain 'command', got %s", name)
	}
}

func TestJSONMarshalerNameFromMessage(t *testing.T) {
	namer := NewShortlinkNamer("test")
	m := NewJSONMarshaler(namer)

	cmd := &testCommand{OrderId: "123"}
	msg, err := m.Marshal(cmd)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	name := m.NameFromMessage(msg)
	if name == "" {
		t.Fatal("name should not be empty")
	}

	typeName := msg.Metadata.Get(MetadataTypeName)
	version := msg.Metadata.Get(MetadataTypeVersion)
	expected := typeName + "." + version

	if name != expected {
		t.Errorf("expected name %s, got %s", expected, name)
	}
}

func TestJSONMarshalerNameFromMessageNil(t *testing.T) {
	m := NewJSONMarshaler(NewShortlinkNamer("test"))

	name := m.NameFromMessage(nil)
	if name != "" {
		t.Errorf("expected empty name for nil message, got %s", name)
	}
}

func TestJSONMarshalerEventName(t *testing.T) {
	namer := NewShortlinkNamer("test")
	m := NewJSONMarshaler(namer)

	evt := &testEvent{OrderId: "123"}
	name := m.Name(evt)

	if !strings.Contains(name, "event") {
		t.Errorf("name should contain 'event', got %s", name)
	}
}


