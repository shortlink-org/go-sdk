package watermill

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

// DLQMessage is stored in <topic>.DLQ
type DLQMessage struct {
	Topic        string            `json:"topic"`
	Payload      json.RawMessage   `json:"payload"`
	Metadata     map[string]string `json:"metadata"`
	Error        string            `json:"error"`
	RetryCount   int               `json:"retry_count"`
	OriginalUUID string            `json:"original_uuid"`
}

const (
	dlqRetryMetadataKey       = "watermill_dlq_retry_count"
	legacyDLQRetryMetadataKey = "retry"
)

// DLQMiddleware sends message to <topic>.DLQ after maxRetries failed attempts.
func DLQMiddleware(pub message.Publisher, maxRetries int) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			if msg.Metadata == nil {
				msg.Metadata = make(map[string]string)
			}

			retryCount := extractRetryCount(msg.Metadata)

			produced, err := h(msg)
			if err == nil {
				return produced, nil
			}

			if maxRetries > 0 && retryCount < maxRetries {
				msg.Metadata.Set(dlqRetryMetadataKey, strconv.Itoa(retryCount+1))
				return produced, err
			}

			topic := msg.Metadata.Get("received_topic")
			if topic == "" {
				topic = msg.Metadata.Get("topic")
			}
			if topic == "" {
				return produced, fmt.Errorf("handler error: %w (missing topic for DLQ)", err)
			}

			payload, marshalErr := json.Marshal(&DLQMessage{
				Topic:        topic,
				Payload:      json.RawMessage(msg.Payload),
				Metadata:     copyMetadata(msg.Metadata),
				Error:        err.Error(),
				RetryCount:   retryCount,
				OriginalUUID: msg.UUID,
			})
			if marshalErr != nil {
				return produced, fmt.Errorf("handler error: %w, dlq marshal error: %v", err, marshalErr)
			}

			dlqMsg := message.NewMessage(watermill.NewUUID(), payload)
			if ctx := msg.Context(); ctx != nil {
				dlqMsg.SetContext(ctx)
				InjectTrace(ctx, dlqMsg)
			}

			if publishErr := pub.Publish(topic+".DLQ", dlqMsg); publishErr != nil {
				return produced, fmt.Errorf("handler error: %w, dlq publish error: %v", err, publishErr)
			}

			return produced, nil
		}
	}
}

func extractRetryCount(metadata message.Metadata) int {
	if metadata == nil {
		return 0
	}
	if v := metadata.Get(dlqRetryMetadataKey); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	if v := metadata.Get(legacyDLQRetryMetadataKey); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return 0
}

func copyMetadata(md message.Metadata) map[string]string {
	if len(md) == 0 {
		return map[string]string{}
	}
	cpy := make(map[string]string, len(md))
	for k, v := range md {
		cpy[k] = v
	}
	return cpy
}
