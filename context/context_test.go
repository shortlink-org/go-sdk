package ctx_test

import (
	"context"
	"errors"
	"testing"

	ctx "github.com/shortlink-org/go-sdk/context"
)

func TestNew(t *testing.T) {
	t.Parallel()

	c, cancel, err := ctx.New()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c == nil {
		t.Fatal("expected non-nil context")
	}

	if cancel == nil {
		t.Fatal("expected non-nil cancel function")
	}

	// Cancel with a cause and verify it is recorded.
	cause := errors.New("test cause")
	cancel(cause)

	if c.Err() != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", c.Err())
	}

	if got := context.Cause(c); !errors.Is(got, cause) {
		t.Fatalf("expected cause %v, got %v", cause, got)
	}
}

func TestNew_NilCause(t *testing.T) {
	t.Parallel()

	c, cancel, err := ctx.New()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cancel with nil cause — cause should be context.Canceled.
	cancel(nil)

	if c.Err() != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", c.Err())
	}

	if got := context.Cause(c); !errors.Is(got, context.Canceled) {
		t.Fatalf("expected cause context.Canceled, got %v", got)
	}
}
