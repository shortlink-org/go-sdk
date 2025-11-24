package watermill

import (
	"context"

	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	MetaTraceID = "otel_trace_id"
	MetaSpanID  = "otel_span_id"
)

// InjectTrace writes OTEL span context into Watermill metadata.
func InjectTrace(ctx context.Context, msg *message.Message) {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return
	}

	msg.Metadata.Set(MetaTraceID, spanCtx.TraceID().String())
	msg.Metadata.Set(MetaSpanID, spanCtx.SpanID().String())
	msg.SetContext(ctx)
}

// ExtractTrace builds ctx from message metadata.
func ExtractTrace(parent context.Context, msg *message.Message) context.Context {
	tid := msg.Metadata.Get(MetaTraceID)
	sid := msg.Metadata.Get(MetaSpanID)

	if tid == "" || sid == "" {
		return parent
	}

	traceID, err1 := trace.TraceIDFromHex(tid)
	spanID, err2 := trace.SpanIDFromHex(sid)
	if err1 != nil || err2 != nil {
		return parent
	}

	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})

	return trace.ContextWithRemoteSpanContext(parent, spanCtx)
}

// OTelMiddleware — tracing middleware.
type OTelMiddleware struct {
	tracer trace.Tracer
}

// NewOTELMiddleware creates OTEL middleware with explicit tracer provider.
func NewOTELMiddleware(provider trace.TracerProvider) *OTelMiddleware {
	return &OTelMiddleware{
		tracer: provider.Tracer("watermill"),
	}
}

// For Router handlers.
func (o *OTelMiddleware) HandlerMiddleware() message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			// Extract context
			parent := msg.Context()
			if parent == nil {
				parent = context.Background()
			}
			ctx := ExtractTrace(parent, msg)

			// Start consumer span
			_, span := o.tracer.Start(ctx, "watermill.consume", trace.WithAttributes(
				attribute.String("topic", msg.Metadata.Get("received_topic")),
			))
			defer span.End()

			// Put context back inside message
			msg.SetContext(ctx)

			return h(msg)
		}
	}
}
