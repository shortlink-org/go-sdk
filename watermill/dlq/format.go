package dlq

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"
	"uuid"

	"github.com/ThreeDotsLabs/watermill/message"
)

// DLQEvent describes the payload stored inside Shortlink DLQ messages.
//
// The JSON field names are snake_case on purpose and must stay that way: they
// are the on-the-wire shape of every dead-letter message, read by consumers
// outside this repository. Renaming them to camelCase would satisfy the linter
// and silently break those readers.
//
//nolint:tagliatelle // snake_case is the published DLQ wire format
type DLQEvent struct {
	FailedAt    time.Time        `json:"failed_at"`
	Reason      string           `json:"reason"`
	OriginalMsg *message.Message `json:"-"`
	Stacktrace  string           `json:"stacktrace,omitempty"`
	ServiceName string           `json:"service_name,omitempty"`
}

// ErrMissingOriginalMessage reports a DLQ event with nothing to carry.
var ErrMissingOriginalMessage = errors.New("dlq event missing original message")

// BuildDLQMessage serializes the DLQEvent and enriches metadata to keep context.
//
//nolint:gocritic // DLQEvent is public API; callers build it by value
func BuildDLQMessage(event DLQEvent) (*message.Message, error) {
	if event.OriginalMsg == nil {
		return nil, ErrMissingOriginalMessage
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
//
//nolint:gocritic // the value receiver is required by json.Marshaler
func (event DLQEvent) MarshalJSON() ([]byte, error) {
	if event.OriginalMsg == nil {
		return nil, ErrMissingOriginalMessage
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

	//nolint:tagliatelle // snake_case is the published DLQ wire format
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

//nolint:tagliatelle // snake_case is the published DLQ wire format
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
	maps.Copy(out, md)

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
