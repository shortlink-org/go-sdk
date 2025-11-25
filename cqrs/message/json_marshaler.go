package message

import (
	"encoding/json"
	"fmt"
	"strings"

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
func (m *JSONMarshaler) Marshal(v any) (*wmmessage.Message, error) {
	payload, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}

	wmMsg := wmmessage.NewMessage(uuid.NewString(), payload)
	ensureMetadata(wmMsg)

	name := m.Name(v)
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

	kind := string(inferKind(v))
	wmMsg.Metadata.Set(MetadataMessageKind, kind)

	return wmMsg, nil
}

// Unmarshal decodes JSON payload into provided value.
func (m *JSONMarshaler) Unmarshal(msg *wmmessage.Message, v any) error {
	if msg == nil {
		return fmt.Errorf("message is nil")
	}
	if len(msg.Payload) == 0 {
		return fmt.Errorf("message payload is empty")
	}

	return json.Unmarshal(msg.Payload, v)
}

// Name returns canonical name for payload.
func (m *JSONMarshaler) Name(v any) string {
	if m != nil && m.namer != nil {
		switch inferKind(v) {
		case KindEvent:
			return m.namer.EventName(v)
		default:
			return m.namer.CommandName(v)
		}
	}
	return NameOf(v)
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
		return strings.Join([]string{typeName, version}, ".")
	}
	return NameOf(msg)
}
