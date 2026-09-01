package message

import (
	"context"
	"fmt"
	"strings"
	"uuid"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"google.golang.org/protobuf/proto"
)

// Marshaler serializes domain messages to Watermill messages.
//
//nolint:iface // exported contract implemented by SDK consumers outside this package.
type Marshaler interface {
	Marshal(ctx context.Context, v any) (*wmmessage.Message, error)
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
func (m *ProtoMarshaler) Marshal(ctx context.Context, value any) (*wmmessage.Message, error) { //nolint:contextcheck // nil-ctx fallback, not a discarded parent
	msg, ok := toProto(value)
	if !ok {
		return nil, fmt.Errorf("%w: got %T", errValueNotProto, value)
	}

	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	wmMsg := wmmessage.NewMessageWithContext(ctx, uuid.New().String(), payload)
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
		wmMsg.Metadata.Set(MetadataContentType, "application/x-protobuf")
	}

	if wmMsg.Metadata.Get(MetadataServiceName) == "" && m.namer != nil {
		wmMsg.Metadata.Set(MetadataServiceName, m.namer.ServiceName())
	}

	kind := string(inferKind(value))
	wmMsg.Metadata.Set(MetadataMessageKind, kind)

	return wmMsg, nil
}

// Unmarshal decodes protobuf payload into provided value.
func (m *ProtoMarshaler) Unmarshal(msg *wmmessage.Message, value any) error {
	if msg == nil {
		return errMessageNil
	}

	if len(msg.Payload) == 0 {
		return errMessageEmptyBody
	}

	protoMsg, ok := value.(proto.Message)
	if !ok {
		return fmt.Errorf("%w: got %T", errTargetNotProto, value)
	}

	return proto.Unmarshal(msg.Payload, protoMsg)
}

// Name returns canonical name for payload.
func (m *ProtoMarshaler) Name(value any) string {
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

		return typeName + "." + version
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
