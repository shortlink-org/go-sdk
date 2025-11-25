package dlq

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
)

// DLQEvent describes the payload stored inside Shortlink DLQ messages.
type DLQEvent struct {
	FailedAt    time.Time        `json:"failed_at"`
	Reason      string           `json:"reason"`
	OriginalMsg *message.Message `json:"-"`
	Stacktrace  string           `json:"stacktrace,omitempty"`
	ServiceName string           `json:"service_name,omitempty"`
}

// BuildDLQMessage serializes the DLQEvent and enriches metadata to keep context.
func BuildDLQMessage(event DLQEvent) (*message.Message, error) {
	if event.OriginalMsg == nil {
		return nil, fmt.Errorf("dlq event missing original message")
	}

	if event.FailedAt.IsZero() {
		event.FailedAt = time.Now().UTC()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal dlq event: %w", err)
	}

	msg := message.NewMessage(uuid.New().String(), payload)

	copyOriginalMetadata(event.OriginalMsg.Metadata, msg)

	msg.Metadata.Set("poison_reason", event.Reason)
	msg.Metadata.Set("poison_stacktrace", event.Stacktrace)
	msg.Metadata.Set("service_name", event.ServiceName)
	msg.Metadata.Set("dlq_version", "1")

	return msg, nil
}

// MarshalJSON customizes the JSON structure to keep original payload and metadata.
func (event DLQEvent) MarshalJSON() ([]byte, error) {
	if event.OriginalMsg == nil {
		return nil, fmt.Errorf("dlq event missing original message")
	}

	original := originalMessageJSON{
		UUID:     event.OriginalMsg.UUID,
		Metadata: copyMetadata(event.OriginalMsg.Metadata),
	}

	switch {
	case len(event.OriginalMsg.Payload) == 0:
		original.Payload = json.RawMessage([]byte("null"))
	case json.Valid(event.OriginalMsg.Payload):
		original.Payload = json.RawMessage(event.OriginalMsg.Payload)
	default:
		original.PayloadBase64 = base64.StdEncoding.EncodeToString(event.OriginalMsg.Payload)
	}

	type alias struct {
		FailedAt    time.Time           `json:"failed_at"`
		Reason      string              `json:"reason"`
		Stacktrace  string              `json:"stacktrace,omitempty"`
		ServiceName string              `json:"service_name,omitempty"`
		Original    originalMessageJSON `json:"original_message"`
	}

	return json.Marshal(alias{
		FailedAt:    event.FailedAt,
		Reason:      event.Reason,
		Stacktrace:  event.Stacktrace,
		ServiceName: event.ServiceName,
		Original:    original,
	})
}

type originalMessageJSON struct {
	UUID          string            `json:"uuid"`
	Metadata      map[string]string `json:"metadata"`
	Payload       json.RawMessage   `json:"payload,omitempty"`
	PayloadBase64 string            `json:"payload_base64,omitempty"`
}

func copyMetadata(md message.Metadata) map[string]string {
	if len(md) == 0 {
		return map[string]string{}
	}

	out := make(map[string]string, len(md))
	for k, v := range md {
		out[k] = v
	}

	return out
}

func copyOriginalMetadata(source message.Metadata, target *message.Message) {
	if target.Metadata == nil {
		target.Metadata = make(map[string]string)
	}

	for k, v := range source {
		target.Metadata.Set("original_"+k, v)
	}
}
