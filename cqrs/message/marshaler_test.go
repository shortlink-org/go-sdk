package message

import (
	"context"
	"testing"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"google.golang.org/protobuf/runtime/protoimpl"
)

type dummyProto struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (*dummyProto) Reset()         {}
func (*dummyProto) String() string { return "dummy" }
func (*dummyProto) ProtoMessage()  {}

func TestProtoMarshalerUnmarshalEmptyPayload(t *testing.T) {
	m := NewProtoMarshaler(NewShortlinkNamer("svc"))
	msg := wmmessage.NewMessageWithContext(context.Background(), "id", nil)

	err := m.Unmarshal(msg, &dummyProto{})
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}
