package message

import (
	"fmt"
	"strings"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// Marshaler serializes domain messages to Watermill messages.
type Marshaler interface {
	Marshal(v any) (*wmmessage.Message, error)
	Unmarshal(msg *wmmessage.Message, v any) error
	Name(v any) string
	NameFromMessage(msg *wmmessage.Message) string
}

// ProtoMarshaler marshals protobuf payloads with Shortlink metadata.
type ProtoMarshaler struct {
	namer Namer
}

// NewProtoMarshaler builds a marshaler that uses provided namer.
func NewProtoMarshaler(namer Namer) *ProtoMarshaler {
	return &ProtoMarshaler{namer: namer}
}

// Marshal encodes protobuf payload and enriches metadata.
func (m *ProtoMarshaler) Marshal(v any) (*wmmessage.Message, error) {
	msg, ok := toProto(v)
	if !ok {
		return nil, fmt.Errorf("value %T does not implement proto.Message", v)
	}

	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
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
		wmMsg.Metadata.Set(MetadataContentType, "application/x-protobuf")
	}
	if wmMsg.Metadata.Get(MetadataServiceName) == "" && m.namer != nil {
		wmMsg.Metadata.Set(MetadataServiceName, m.namer.ServiceName())
	}

	kind := string(inferKind(v))
	wmMsg.Metadata.Set(MetadataMessageKind, kind)

	return wmMsg, nil
}

// Unmarshal decodes protobuf payload into provided value.
func (m *ProtoMarshaler) Unmarshal(msg *wmmessage.Message, v any) error {
	if msg == nil {
		return fmt.Errorf("message is nil")
	}
	if len(msg.Payload) == 0 {
		return fmt.Errorf("message payload is empty")
	}

	protoMsg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("target %T does not implement proto.Message", v)
	}

	return proto.Unmarshal(msg.Payload, protoMsg)
}

// Name returns canonical name for payload.
func (m *ProtoMarshaler) Name(v any) string {
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
func (m *ProtoMarshaler) NameFromMessage(msg *wmmessage.Message) string {
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

func splitCanonicalName(full string) (string, string) {
	if full == "" {
		return "", defaultVersion
	}
	parts := strings.Split(full, ".")
	if len(parts) <= 1 {
		return full, defaultVersion
	}
	version := parts[len(parts)-1]
	typeName := strings.Join(parts[:len(parts)-1], ".")
	if version == "" {
		version = defaultVersion
	}
	return typeName, version
}
