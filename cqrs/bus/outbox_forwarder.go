package bus

import (
	"context"
	"log/slog"
	"sync"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/components/forwarder"
	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/shortlink-org/go-sdk/logger"
	sdkwatermill "github.com/shortlink-org/go-sdk/watermill"
)

type forwarderState struct {
	cfg      *OutboxConfig
	once     sync.Once
	fwd      *forwarder.Forwarder
	err      error
	monitor  *forwarderMonitor
	wmLogger watermill.LoggerAdapter
}

func newForwarderState(cfg *OutboxConfig) *forwarderState {
	if cfg == nil {
		return nil
	}

	copy := *cfg

	state := &forwarderState{
		cfg:      &copy,
		monitor:  newForwarderMonitor(copy.Logger, copy.MeterProvider, copy.ForwarderName),
		wmLogger: sdkwatermill.NewWatermillLogger(copy.Logger),
	}

	return state
}

func (s *forwarderState) wrapPublisher(pub wmmessage.Publisher) wmmessage.Publisher {
	if s == nil || s.cfg == nil || pub == nil {
		return pub
	}
	pubCfg := forwarder.PublisherConfig{
		ForwarderTopic: s.cfg.ForwarderName,
	}
	return forwarder.NewPublisher(pub, pubCfg)
}

func (s *forwarderState) ensureForwarder() (*forwarder.Forwarder, error) {
	if s == nil || s.cfg == nil {
		return nil, nil
	}

	s.once.Do(func() {
		var middlewares []wmmessage.HandlerMiddleware
		if mw := s.monitor.middleware(); mw != nil {
			middlewares = append(middlewares, mw)
		}

		forwarderCfg := forwarder.Config{
			ForwarderTopic: s.cfg.ForwarderName,
			Middlewares:    middlewares,
		}

		s.fwd, s.err = forwarder.NewForwarder(
			s.cfg.Subscriber,
			s.cfg.RealPublisher,
			s.wmLogger,
			forwarderCfg,
		)
	})

	return s.fwd, s.err
}

func (s *forwarderState) Run(ctx context.Context) error {
	if s == nil || s.cfg == nil {
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	s.cfg.Logger.Info("Starting CQRS outbox forwarder",
		slog.String("forwarder", s.cfg.ForwarderName),
	)

	fwd, err := s.ensureForwarder()
	if err != nil || fwd == nil {
		if err != nil {
			s.cfg.Logger.Error("Failed to initialize outbox forwarder",
				slog.String("forwarder", s.cfg.ForwarderName),
				slog.String("error", err.Error()),
			)
		}
		return err
	}

	if err := fwd.Run(ctx); err != nil {
		s.cfg.Logger.Error("Outbox forwarder stopped with error",
			slog.String("forwarder", s.cfg.ForwarderName),
			slog.String("error", err.Error()),
		)
		return err
	}

	s.cfg.Logger.Info("Outbox forwarder stopped",
		slog.String("forwarder", s.cfg.ForwarderName),
	)

	return nil
}

func (s *forwarderState) Close(ctx context.Context) error {
	if s == nil || s.cfg == nil {
		return nil
	}

	fwd, err := s.ensureForwarder()
	if err != nil || fwd == nil {
		return err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	done := make(chan error, 1)
	go func() {
		done <- fwd.Close()
	}()

	select {
	case <-ctx.Done():
		s.cfg.Logger.Warn("Outbox forwarder close interrupted by context",
			slog.String("forwarder", s.cfg.ForwarderName),
			slog.String("error", ctx.Err().Error()),
		)
		return ctx.Err()
	case err := <-done:
		if err != nil {
			s.cfg.Logger.Error("Failed to close outbox forwarder",
				slog.String("forwarder", s.cfg.ForwarderName),
				slog.String("error", err.Error()),
			)
			return err
		}

		s.cfg.Logger.Info("Outbox forwarder closed",
			slog.String("forwarder", s.cfg.ForwarderName),
		)

		return nil
	}
}

type forwarderMonitor struct {
	log           logger.Logger
	forwarderName string
	success       metric.Int64Counter
	failures      metric.Int64Counter
	attrs         []attribute.KeyValue
}

func newForwarderMonitor(log logger.Logger, provider metric.MeterProvider, name string) *forwarderMonitor {
	if log == nil || provider == nil {
		return nil
	}

	meter := provider.Meter("shortlink.cqrs.outbox")

	var (
		success metric.Int64Counter
		fail    metric.Int64Counter
		err     error
	)

	if success, err = meter.Int64Counter(
		"shortlink_cqrs_outbox_forwarded_total",
		metric.WithDescription("Total number of successfully forwarded messages"),
	); err != nil {
		log.Warn("Failed to create CQRS outbox success counter", slog.String("error", err.Error()))
	}

	if fail, err = meter.Int64Counter(
		"shortlink_cqrs_outbox_failed_total",
		metric.WithDescription("Total number of failed forwarder deliveries"),
	); err != nil {
		log.Warn("Failed to create CQRS outbox failure counter", slog.String("error", err.Error()))
	}

	return &forwarderMonitor{
		log:           log,
		forwarderName: name,
		success:       success,
		failures:      fail,
		attrs: []attribute.KeyValue{
			attribute.String("forwarder_name", name),
		},
	}
}

func (m *forwarderMonitor) middleware() wmmessage.HandlerMiddleware {
	if m == nil {
		return nil
	}

	return func(next wmmessage.HandlerFunc) wmmessage.HandlerFunc {
		return func(msg *wmmessage.Message) ([]*wmmessage.Message, error) {
			result, err := next(msg)

			ctx := msg.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			if err != nil {
				m.observeFailure(ctx, msg, err)
				return result, err
			}

			m.observeSuccess(ctx, msg)
			return result, nil
		}
	}
}

func (m *forwarderMonitor) observeSuccess(ctx context.Context, msg *wmmessage.Message) {
	if m == nil {
		return
	}

	if m.success != nil {
		m.success.Add(ctx, 1, metric.WithAttributes(m.attrs...))
	}

	m.log.DebugWithContext(ctx, "CQRS outbox forwarded message",
		slog.String("forwarder", m.forwarderName),
		slog.String("message_uuid", msg.UUID),
	)
}

func (m *forwarderMonitor) observeFailure(ctx context.Context, msg *wmmessage.Message, err error) {
	if m == nil {
		return
	}

	if m.failures != nil {
		m.failures.Add(ctx, 1, metric.WithAttributes(m.attrs...))
	}

	m.log.ErrorWithContext(ctx, "CQRS outbox failed to forward message",
		slog.String("forwarder", m.forwarderName),
		slog.String("message_uuid", msg.UUID),
		slog.String("error", err.Error()),
	)
}
