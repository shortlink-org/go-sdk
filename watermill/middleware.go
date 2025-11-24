package watermill

import (
	"context"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	wmmid "github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// ----------- BASE MIDDLEWARE (panic, correlation, retry) ------------

func configureBaseMiddlewares(router *message.Router, cfg *config.Config, log logger.Logger) {
	router.AddMiddleware(wmmid.Recoverer)
	router.AddMiddleware(wmmid.CorrelationID)

	// Configure retry middleware from config
	cfg.SetDefault("WATERMILL_RETRY_MAX_RETRIES", 3)
	cfg.SetDefault("WATERMILL_RETRY_INITIAL_INTERVAL", "150ms")
	cfg.SetDefault("WATERMILL_RETRY_MAX_INTERVAL", "2s")
	cfg.SetDefault("WATERMILL_RETRY_MULTIPLIER", 2.0)

	maxRetries := cfg.GetInt("WATERMILL_RETRY_MAX_RETRIES")
	initialInterval := cfg.GetDuration("WATERMILL_RETRY_INITIAL_INTERVAL")
	maxInterval := cfg.GetDuration("WATERMILL_RETRY_MAX_INTERVAL")
	multiplier := cfg.GetFloat64("WATERMILL_RETRY_MULTIPLIER")

	router.AddMiddleware(
		wmmid.Retry{
			MaxRetries:      maxRetries,
			InitialInterval: initialInterval,
			MaxInterval:     maxInterval,
			Multiplier:      multiplier,
		}.Middleware,
	)

	log.Info("Configured retry middleware",
		slog.Int("max_retries", maxRetries),
		slog.String("initial_interval", initialInterval.String()),
		slog.String("max_interval", maxInterval.String()),
		slog.Float64("multiplier", multiplier),
	)
}

// -------------------- METRICS MIDDLEWARE ---------------------------

type MetricsMiddleware struct {
	meter metric.Meter

	published metric.Int64Counter
	consumed  metric.Int64Counter
	errors    metric.Int64Counter

	pubLatency metric.Float64Histogram
	conLatency metric.Float64Histogram
}

// NewMetricsMiddleware creates metrics middleware with explicit meter provider.
func NewMetricsMiddleware(log logger.Logger, provider metric.MeterProvider) (*MetricsMiddleware, error) {
	m := provider.Meter("watermill")

	pub, err := m.Int64Counter(
		"watermill_messages_published_total",
		metric.WithDescription("Total number of messages published to topics"),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Error("Failed to create published counter metric", slog.String("error", err.Error()))
		return nil, err
	}

	cons, err := m.Int64Counter(
		"watermill_messages_consumed_total",
		metric.WithDescription("Total number of messages consumed from topics"),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Error("Failed to create consumed counter metric", slog.String("error", err.Error()))
		return nil, err
	}

	errc, err := m.Int64Counter(
		"watermill_messages_failed_total",
		metric.WithDescription("Total number of failed message operations (publish or consume)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Error("Failed to create errors counter metric", slog.String("error", err.Error()))
		return nil, err
	}

	pubLat, err := m.Float64Histogram(
		"watermill_publish_latency_seconds",
		metric.WithDescription("Latency of message publishing operations in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		log.Error("Failed to create publish latency histogram metric", slog.String("error", err.Error()))
		return nil, err
	}

	conLat, err := m.Float64Histogram(
		"watermill_consume_latency_seconds",
		metric.WithDescription("Latency of message consumption operations in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		log.Error("Failed to create consume latency histogram metric", slog.String("error", err.Error()))
		return nil, err
	}

	return &MetricsMiddleware{
		meter:      m,
		published:  pub,
		consumed:   cons,
		errors:     errc,
		pubLatency: pubLat,
		conLatency: conLat,
	}, nil
}

// Handler middleware — measure consumption latency + exemplar support.
func (m *MetricsMiddleware) HandlerMiddleware() message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			start := time.Now()

			topic := msg.Metadata.Get("received_topic")
			ctx := msg.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			attrs := metric.WithAttributes(attribute.String("topic", topic))

			msgs, err := h(msg)
			lat := time.Since(start).Seconds()

			if err != nil {
				m.errors.Add(ctx, 1, metric.WithAttributes(m.errorAttributes(topic, "consume", err)...))
				return msgs, err
			}

			m.consumed.Add(ctx, 1, attrs)
			m.conLatency.Record(ctx, lat, attrs)

			return msgs, nil
		}
	}
}

// PublisherWrapper — adds metrics + exemplars to publisher.
func (m *MetricsMiddleware) PublisherWrapper(pub message.Publisher, otelMW *OTelMiddleware) message.Publisher {
	return &publisherWrapper{
		pub:     pub,
		metrics: m,
		otel:    otelMW,
	}
}

type publisherWrapper struct {
	pub     message.Publisher
	metrics *MetricsMiddleware
	otel    *OTelMiddleware
}

func (pw *publisherWrapper) Publish(topic string, msgs ...*message.Message) error {
	return pw.publishWithMetrics(topic, msgs...)
}

func (pw *publisherWrapper) Close() error {
	return pw.pub.Close()
}

func (pw *publisherWrapper) publishWithMetrics(topic string, msgs ...*message.Message) error {
	// Extract context before publishing for proper tracing
	ctx := context.Background()
	if len(msgs) > 0 && msgs[0].Context() != nil {
		ctx = msgs[0].Context()
	}

	start := time.Now()

	var span trace.Span
	if pw.otel != nil {
		ctx, span = pw.otel.tracer.Start(ctx, "watermill.publish", trace.WithAttributes(
			attribute.String("topic", topic),
		))
		defer span.End()
	}

	// inject trace metadata on publish using span-aware context
	for _, msg := range msgs {
		msg.SetContext(ctx)
		InjectTrace(ctx, msg)
	}

	err := pw.pub.Publish(topic, msgs...)
	lat := time.Since(start).Seconds()

	attrs := metric.WithAttributes(attribute.String("topic", topic))

	if err != nil {
		if span != nil {
			span.RecordError(err)
		}
		pw.metrics.errors.Add(ctx, 1, metric.WithAttributes(pw.metrics.errorAttributes(topic, "publish", err)...))
		return err
	}

	pw.metrics.published.Add(ctx, int64(len(msgs)), attrs)
	pw.metrics.pubLatency.Record(ctx, lat, attrs)

	return nil
}

// Helper: extract trace/span id from ctx for exemplars.
func TraceID(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ""
	}
	return spanCtx.TraceID().String()
}

func SpanID(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ""
	}
	return spanCtx.SpanID().String()
}

const metricErrorMaxLen = 128

func (m *MetricsMiddleware) errorAttributes(topic, stage string, err error) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("topic", topic),
		attribute.String("stage", stage),
	}
	if err == nil {
		return attrs
	}
	errStr := err.Error()
	if len(errStr) > metricErrorMaxLen {
		errStr = errStr[:metricErrorMaxLen]
	}
	attrs = append(attrs, attribute.String("error", errStr))
	return attrs
}
