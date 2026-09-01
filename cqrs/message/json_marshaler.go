package message

import (
	"context"
	"encoding/json"
	"fmt"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
)

// JSONMarshaler marshals JSON payloads with Shortlink metadata.
type JSONMarshaler struct {
	namer Namer
}

// NewJSONMarshaler builds a marshaler that uses provided namer.
func NewJSONMarshaler(namer Namer) *JSONMarshaler {
	return &JSONMarshaler{namer: namer}
}

// Marshal encodes JSON payload and enriches metadata.
func (m *JSONMarshaler) Marshal(ctx context.Context, value any) (*wmmessage.Message, error) { //nolint:contextcheck // nil-ctx fallback, not a discarded parent
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	wmMsg := wmmessage.NewMessageWithContext(ctx, uuid.NewString(), payload)
	ensureMetadata(wmMsg)

	name := m.Name(value)
	typeName, version := splitCanonicalName(name)

	if wmMsg.Metadata.Get(MetadataTypeName) == "" {
		wmMsg.Metadata.Set(MetadataTypeName, typeName)
	}

	if wmMsg.Metadata.Get(MetadataTypeVersion) == "" {
		wmMsg.Metadata.Set(MetadataTypeVersion, version)
	}

	if wmMsg.Metadata.Get(MetadataContentType) == "" {
		wmMsg.Metadata.Set(MetadataContentType, "application/json")
	}

	if wmMsg.Metadata.Get(MetadataServiceName) == "" && m.namer != nil {
		wmMsg.Metadata.Set(MetadataServiceName, m.namer.ServiceName())
	}

	kind := string(inferKind(value))
	wmMsg.Metadata.Set(MetadataMessageKind, kind)

	return wmMsg, nil
}

// Unmarshal decodes JSON payload into provided value.
func (m *JSONMarshaler) Unmarshal(msg *wmmessage.Message, value any) error {
	if msg == nil {
		return errMessageNil
	}

	if len(msg.Payload) == 0 {
		return errMessageEmptyBody
	}

	return json.Unmarshal(msg.Payload, value)
}

// Name returns canonical name for payload.
func (m *JSONMarshaler) Name(value any) string {
	if m != nil && m.namer != nil {
		switch inferKind(value) {
		case KindEvent:
			return m.namer.EventName(value)
		default:
			return m.namer.CommandName(value)
		}
	}

	return NameOf(value)
}

// NameFromMessage reconstructs canonical name using message metadata.
func (m *JSONMarshaler) NameFromMessage(msg *wmmessage.Message) string {
	if msg == nil {
		return ""
	}

	typeName := msg.Metadata.Get(MetadataTypeName)

	version := msg.Metadata.Get(MetadataTypeVersion)
	if typeName != "" {
		if version == "" {
			version = defaultVersion
		}

		return typeName + "." + version
	}

	return NameOf(msg)
}
