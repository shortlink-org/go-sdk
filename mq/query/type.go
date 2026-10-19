package query

import (
	"context"
)

type ResponseMessage struct {
	Context context.Context //nolint:containedctx // public API: carries the trace context the driver extracted from the message headers
	Body    []byte
}

type Response struct {
	Chan chan ResponseMessage
}
