package message

import (
	"context"
	"os"
	"strings"
	"time"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var (
	metadataNamespace = func() string {
		ns := strings.TrimSpace(os.Getenv("SHORTLINK_METADATA_NAMESPACE"))
		if ns == "" {
			ns = "shortlink"
		}
		return strings.ToLower(ns)
	}()
	MetadataTraceID     = metadataKey("trace_id")
	MetadataSpanID      = metadataKey("span_id")
	MetadataServiceName = metadataKey("service_name")
	MetadataTypeName    = metadataKey("type_name")
	MetadataTypeVersion = metadataKey("type_version")
	MetadataContentType = metadataKey("content_type")
	MetadataOccurredAt  = metadataKey("occurred_at")
	MetadataMessageKind = metadataKey("message_kind")
)

func metadataKey(suffix string) string {
	return metadataNamespace + "." + suffix
}

type ctxKey string

const (
	serviceNameKey ctxKey = "shortlink.service_name_ctx"
)

func ensureMetadata(msg *wmmessage.Message) {
	if msg.Metadata == nil {
		msg.Metadata = make(wmmessage.Metadata)
	}
}

// WithServiceName stores service name inside context for downstream metadata injection.
func WithServiceName(ctx context.Context, serviceName string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if serviceName == "" {
		return ctx
	}
	return context.WithValue(ctx, serviceNameKey, serviceName)
}

// ServiceNameFromContext extracts service name used to enrich message metadata.
func ServiceNameFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if val, ok := ctx.Value(serviceNameKey).(string); ok {
		return val
	}
	return ""
}

// SetTrace injects tracing metadata and propagates OTEL headers through Watermill message.
func SetTrace(ctx context.Context, msg *wmmessage.Message) {
	if msg == nil {
		return
	}

	if ctx == nil {
		ctx = context.Background()
	}

	ensureMetadata(msg)

	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		msg.Metadata.Set(MetadataTraceID, spanCtx.TraceID().String())
		msg.Metadata.Set(MetadataSpanID, spanCtx.SpanID().String())
	}

	// Preserve service name if it was configured earlier.
	if service := ServiceNameFromContext(ctx); service != "" && msg.Metadata.Get(MetadataServiceName) == "" {
		msg.Metadata.Set(MetadataServiceName, service)
	}

	if msg.Metadata.Get(MetadataOccurredAt) == "" {
		msg.Metadata.Set(MetadataOccurredAt, time.Now().UTC().Format(time.RFC3339Nano))
	}

	msg.SetContext(ctx)

	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(msg.Metadata))
}

// CopyMetadata duplicates metadata map into destination map.
func CopyMetadata(dst, src wmmessage.Metadata) wmmessage.Metadata {
	if src == nil {
		return dst
	}

	if dst == nil {
		dst = make(wmmessage.Metadata, len(src))
	}

	for k, v := range src {
		dst.Set(k, v)
	}

	return dst
}
