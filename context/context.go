package ctx

import (
	"context"
)

func New() (context.Context, context.CancelCauseFunc, error) {
	ctx, cancel := context.WithCancelCause(context.Background())

	return ctx, cancel, nil
}
