package saga

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/saga/dag"
)

type Step struct {
	Options
	ctx    context.Context
	then   func(ctx context.Context) error
	reject func(ctx context.Context, thenError error) error
	dag    *dag.Dag
	name   string
	status StepState
}

func (s *Step) Run() error {
	// add event to parent saga span instead of creating a new span
	span := trace.SpanFromContext(s.ctx)
	if span != nil && span.SpanContext().IsValid() {
		span.AddEvent("saga.step", trace.WithAttributes(
			attribute.String("step", s.name),
			attribute.String("status", "run"),
		))
	}

	s.status = RUN

	err := s.then(s.ctx)
	if err != nil {
		s.status = REJECT

		// set tracing error
		if span != nil && span.SpanContext().IsValid() {
			span.RecordError(err)
			span.AddEvent("saga.step.error", trace.WithAttributes(
				attribute.String("step", s.name),
				attribute.String("status", "reject"),
				attribute.String("error", err.Error()),
			))
		}

		// save error in context
		s.ctx = WithError(s.ctx, err)

		return err
	}

	if span != nil && span.SpanContext().IsValid() {
		span.AddEvent("saga.step.done", trace.WithAttributes(
			attribute.String("step", s.name),
			attribute.String("status", "done"),
		))
	}

	s.status = DONE

	return nil
}

func (s *Step) Reject() error {
	// add event to parent saga span instead of creating a new span
	span := trace.SpanFromContext(s.ctx)
	if span != nil && span.SpanContext().IsValid() {
		span.AddEvent("saga.step.reject", trace.WithAttributes(
			attribute.String("step", s.name),
			attribute.String("status", "reject"),
		))
	}

	s.status = REJECT

	// Check on a compensation step
	if s.reject == nil {
		return nil
	}

	// Get error from context
	thenErr := GetError(s.ctx)

	err := s.reject(s.ctx, thenErr)
	if err != nil {
		if span != nil && span.SpanContext().IsValid() {
			span.RecordError(err)
			span.AddEvent("saga.step.reject.error", trace.WithAttributes(
				attribute.String("step", s.name),
				attribute.String("status", "fail"),
				attribute.String("error", err.Error()),
			))
		}

		s.status = FAIL

		return err
	}

	if span != nil && span.SpanContext().IsValid() {
		span.AddEvent("saga.step.rollback", trace.WithAttributes(
			attribute.String("step", s.name),
			attribute.String("status", "rollback"),
		))
	}

	s.status = ROLLBACK

	return nil
}
