package dlq

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/require"
)

type dlqPayload struct {
	Original struct {
		Payload  json.RawMessage   `json:"payload"`
		Metadata map[string]string `json:"metadata"`
	} `json:"original_message"`
}

func TestBuildDLQMessagePreservesMetadata(t *testing.T) {
	original := message.NewMessage("original-123", []byte(`{"foo":"bar"}`))
	original.Metadata.Set("received_topic", "orders")
	original.Metadata.Set("custom", "meta")

	event := DLQEvent{
		FailedAt:    time.Unix(1, 0).UTC(),
		Reason:      "boom",
		OriginalMsg: original,
		Stacktrace:  "stack",
		ServiceName: "svc",
	}

	msg, err := BuildDLQMessage(event)
	require.NoError(t, err)

	require.Equal(t, "boom", msg.Metadata.Get("poison_reason"))
	require.Equal(t, "stack", msg.Metadata.Get("poison_stacktrace"))
	require.Equal(t, "svc", msg.Metadata.Get("service_name"))
	require.Equal(t, "1", msg.Metadata.Get("dlq_version"))

	require.Equal(t, "orders", msg.Metadata.Get("original_received_topic"))
	require.Equal(t, "meta", msg.Metadata.Get("original_custom"))
}

func TestDLQEventJSONIncludesOriginalPayload(t *testing.T) {
	original := message.NewMessage("original-456", []byte(`{"hello":"world"}`))
	event := DLQEvent{
		FailedAt:    time.Unix(2, 0).UTC(),
		Reason:      "boom",
		OriginalMsg: original,
	}

	msg, err := BuildDLQMessage(event)
	require.NoError(t, err)

	var payload dlqPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &payload))

	require.JSONEq(t, `{"hello":"world"}`, string(payload.Original.Payload))
	require.Equal(t, map[string]string(original.Metadata), payload.Original.Metadata)
}
