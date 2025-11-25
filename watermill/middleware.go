package watermill

import (
	"context"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	wmmid "github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// ----------- BASE MIDDLEWARE (panic, correlation, retry) ------------

func configureBaseMiddlewares(router *message.Router, log logger.Logger, wmLogger watermill.LoggerAdapter, opts Options) {
	router.AddMiddleware(wmmid.Recoverer)
	router.AddMiddleware(wmmid.CorrelationID)

	if opts.Timeout.Enabled {
		router.AddMiddleware(wmmid.Timeout(opts.Timeout.Duration))
		log.Info("Configured timeout middleware",
			slog.String("duration", opts.Timeout.Duration.String()),
		)
	}

	if opts.CircuitBreaker.Enabled {
		cb := wmmid.NewCircuitBreaker(opts.CircuitBreaker.Settings)
		router.AddMiddleware(cb.Middleware)
		log.Info("Configured circuit breaker middleware",
			slog.String("name", opts.CircuitBreaker.Settings.Name),
			slog.String("timeout", opts.CircuitBreaker.Settings.Timeout.String()),
			slog.Uint64("max_requests", uint64(opts.CircuitBreaker.Settings.MaxRequests)),
		)
	}

	if opts.Retry.Enabled {
		retryMiddleware := wmmid.Retry{
			MaxRetries:        opts.Retry.MaxRetries,
			InitialInterval:   opts.Retry.InitialInterval,
			MaxInterval:       opts.Retry.MaxInterval,
			Multiplier:        opts.Retry.Multiplier,
			MaxElapsedTime:    opts.Retry.MaxElapsedTime,
			RandomizationFactor: opts.Retry.Jitter,
			ResetContextOnRetry: opts.Retry.ResetContextOnRetry,
			Logger:             wmLogger,
		}
		router.AddMiddleware(retryMiddleware.Middleware)

		log.Info("Configured retry middleware",
			slog.Int("max_retries", opts.Retry.MaxRetries),
			slog.String("initial_interval", opts.Retry.InitialInterval.String()),
			slog.String("max_interval", opts.Retry.MaxInterval.String()),
			slog.Float64("multiplier", opts.Retry.Multiplier),
			slog.Float64("jitter", opts.Retry.Jitter),
		)
	}
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

			attrs := metric.WithAttributes(m.topicAttributes(ctx, topic)...)

			msgs, err := h(msg)
			lat := time.Since(start).Seconds()

			if err != nil {
				m.errors.Add(ctx, 1, metric.WithAttributes(m.errorAttributes(ctx, topic, "consume", err)...))
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

	attrs := metric.WithAttributes(pw.metrics.topicAttributes(ctx, topic)...)

	if err != nil {
		if span != nil {
			span.RecordError(err)
		}
		pw.metrics.errors.Add(ctx, 1, metric.WithAttributes(pw.metrics.errorAttributes(ctx, topic, "publish", err)...))
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

func (m *MetricsMiddleware) topicAttributes(ctx context.Context, topic string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.String("topic", topic)}
	if traceID := TraceID(ctx); traceID != "" {
		attrs = append(attrs, attribute.String("trace_id", traceID))
	}
	if spanID := SpanID(ctx); spanID != "" {
		attrs = append(attrs, attribute.String("span_id", spanID))
	}
	return attrs
}

func (m *MetricsMiddleware) errorAttributes(ctx context.Context, topic, stage string, err error) []attribute.KeyValue {
	attrs := m.topicAttributes(ctx, topic)
	attrs = append(attrs, attribute.String("stage", stage))
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
