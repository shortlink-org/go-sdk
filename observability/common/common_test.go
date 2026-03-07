package common

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

func TestNewResourceUsesCompatibleSchema(t *testing.T) {
	t.Parallel()

	res, err := NewResource(context.Background(), "shortlink-shop-oms", "dev")
	if err != nil {
		t.Fatalf("NewResource() returned error: %v", err)
	}

	if got, want := res.SchemaURL(), semconv.SchemaURL; got != want {
		t.Fatalf("SchemaURL() = %q, want %q", got, want)
	}

	if _, ok := res.Set().Value(semconv.ServiceNameKey); !ok {
		t.Fatalf("resource missing %q", semconv.ServiceNameKey)
	}

	if _, ok := res.Set().Value(semconv.ServiceVersionKey); !ok {
		t.Fatalf("resource missing %q", semconv.ServiceVersionKey)
	}

	if _, err := resource.Merge(resource.Empty(), res); err != nil {
		t.Fatalf("resource should merge cleanly: %v", err)
	}
}
