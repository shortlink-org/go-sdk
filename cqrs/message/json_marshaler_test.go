package message

import (
	"context"
	"strings"
	"testing"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
)

const testAmount = 99.99

type testCommand struct {
	OrderID string  `json:"orderId"`
	Amount  float64 `json:"amount"`
}

type testEvent struct {
	OrderID   string `json:"orderId"`
	CreatedAt int64  `json:"createdAt"`
}

func TestJSONMarshalerMarshal(t *testing.T) {
	namer := NewShortlinkNamer("test")
	m := NewJSONMarshaler(namer)

	cmd := &testCommand{
		OrderID: "order-123",
		Amount:  testAmount,
	}

	msg, err := m.Marshal(context.Background(), cmd)
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
		OrderID: "order-123",
		Amount:  testAmount,
	}

	msg, err := m.Marshal(context.Background(), original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var unmarshaled testCommand

	err = m.Unmarshal(msg, &unmarshaled)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if unmarshaled.OrderID != original.OrderID {
		t.Errorf("expected OrderID %s, got %s", original.OrderID, unmarshaled.OrderID)
	}

	if unmarshaled.Amount != original.Amount {
		t.Errorf("expected Amount %f, got %f", original.Amount, unmarshaled.Amount)
	}
}

func TestJSONMarshalerUnmarshalEmptyPayload(t *testing.T) {
	m := NewJSONMarshaler(NewShortlinkNamer("svc"))
	msg := wmmessage.NewMessageWithContext(context.Background(), "id", nil)

	var cmd testCommand

	err := m.Unmarshal(msg, &cmd)
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestJSONMarshalerUnmarshalNilMessage(t *testing.T) {
	m := NewJSONMarshaler(NewShortlinkNamer("svc"))

	var cmd testCommand

	err := m.Unmarshal(nil, &cmd)
	if err == nil {
		t.Fatal("expected error for nil message")
	}
}

func TestJSONMarshalerName(t *testing.T) {
	namer := NewShortlinkNamer("test")
	m := NewJSONMarshaler(namer)

	cmd := &testCommand{OrderID: "123"}
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

	cmd := &testCommand{OrderID: "123"}

	msg, err := m.Marshal(context.Background(), cmd)
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

	evt := &testEvent{OrderID: "123"}
	name := m.Name(evt)

	if !strings.Contains(name, "event") {
		t.Errorf("name should contain 'event', got %s", name)
	}
}
