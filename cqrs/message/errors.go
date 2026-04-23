package message

import "errors"

var (
	errMessageNil        = errors.New("cqrs/message: message is nil")
	errMessageEmptyBody  = errors.New("cqrs/message: message payload is empty")
	errValueNotProto     = errors.New("cqrs/message: value does not implement proto.Message")
	errTargetNotProto    = errors.New("cqrs/message: target does not implement proto.Message")
)
