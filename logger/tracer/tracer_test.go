package tracer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/shortlink-org/go-sdk/logger/tracer"
)

func TestZapFieldsToOpenTelemetry(t *testing.T) {
	tests := []struct {
		name   string
		fields []any
		want   []attribute.KeyValue
	}{
		{
			name:   "StringField",
			fields: []any{"key", "value"},
			want:   []attribute.KeyValue{attribute.String("key", "value")},
		},
		{
			name:   "BoolField",
			fields: []any{"flag", true},
			want:   []attribute.KeyValue{attribute.Bool("flag", true)},
		},
		{
			name:   "ErrorField",
			fields: []any{"err", assert.AnError},
			want:   []attribute.KeyValue{attribute.String("err", assert.AnError.Error())},
		},
		{
			name:   "NilErrorField",
			fields: []any{"err", nil},
			want:   []attribute.KeyValue{attribute.String("err", "")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tracer.FieldsToOpenTelemetry(tt.fields...)
			require.Equal(t, tt.want, got)
		})
	}
}
