package message

import (
	"testing"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
)

type createInvoiceCommand struct{}
type invoiceCreatedEvent struct{}

func TestShortlinkNamerCommandName(t *testing.T) {
	namer := NewShortlinkNamer("Billing")

	name := namer.CommandName(&createInvoiceCommand{})
	if name != "billing.command.create_invoice_command.v1" {
		t.Fatalf("unexpected command name: %s", name)
	}

	eventName := namer.EventName(&invoiceCreatedEvent{})
	if eventName != "billing.event.invoice_created_event.v1" {
		t.Fatalf("unexpected event name: %s", eventName)
	}

	if topic := namer.TopicForCommand(name); topic != "billing.command.create_invoice_command.v1" {
		t.Fatalf("unexpected command topic: %s", topic)
	}
	if topic := namer.TopicForEvent(eventName); topic != "billing.event.invoice_created_event.v1" {
		t.Fatalf("unexpected event topic: %s", topic)
	}
}

func TestNameOfUsesMetadata(t *testing.T) {
	env := CommandEnvelope{
		Metadata: map[string]string{
			MetadataTypeName:    "billing.command.generate_invoice",
			MetadataTypeVersion: "v7",
		},
	}

	if got := NameOf(env); got != "billing.command.generate_invoice.v7" {
		t.Fatalf("expected metadata name, got %s", got)
	}

	msg := wmmessage.NewMessage("1", []byte("payload"))
	msg.Metadata.Set(MetadataTypeName, "billing.event.invoice_generated")
	msg.Metadata.Set(MetadataTypeVersion, "v2")
	if got := NameOf(msg); got != "billing.event.invoice_generated.v2" {
		t.Fatalf("expected metadata-derived name, got %s", got)
	}
}

func TestTopicForCommandSanitizes(t *testing.T) {
	name := "Billing.Command.Create Invoice.v1"
	topic := TopicForCommand(name)
	if topic != "billing.command.create_invoice.v1" {
		t.Fatalf("unexpected sanitized topic: %s", topic)
	}
}

func TestInferKindDefaultsToCommand(t *testing.T) {
	if got := inferKind(&createInvoiceCommand{}); got != KindCommand {
		t.Fatalf("expected KindCommand, got %s", got)
	}

	meta := map[string]string{
		MetadataMessageKind: string(KindEvent),
	}
	if got := inferKind(meta); got != KindEvent {
		t.Fatalf("expected KindEvent, got %s", got)
	}
}
