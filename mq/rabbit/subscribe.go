package rabbit

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"

	"github.com/shortlink-org/go-sdk/mq/query"
)

// Subscribe binds a durable queue to a fanout exchange named target, then delivers messages to message.Chan.
// The exchange is created if missing (durable fanout). Publish must use the same exchange name as target with any routing key.
// Returns immediately; a background goroutine reads deliveries until ctx is done or the subscription channel is closed.
func (mq *MQ) Subscribe(ctx context.Context, target string, message query.Response) error {
	mq.mu.Lock()
	if _, exists := mq.subs[target]; exists {
		mq.mu.Unlock()
		return nil
	}

	subCh, err := mq.conn.Channel()
	if err != nil {
		mq.mu.Unlock()
		return fmt.Errorf("rabbit subscribe channel: %w", err)
	}

	queueName := fmt.Sprintf("%s-%s", target, mq.cfg.GetString("SERVICE_NAME"))

	err = subCh.ExchangeDeclare(
		target,
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = subCh.Close()
		mq.mu.Unlock()

		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	q, err := subCh.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = subCh.Close()
		mq.mu.Unlock()

		return fmt.Errorf("failed to declare queue: %w", err)
	}

	err = subCh.QueueBind(q.Name, "", target, false, nil)
	if err != nil {
		_ = subCh.Close()
		mq.mu.Unlock()

		return fmt.Errorf("failed to bind queue: %w", err)
	}

	msgs, err := subCh.Consume(ctx, q.Name, "", true, false, false, false, nil)
	if err != nil {
		_ = subCh.Close()
		mq.mu.Unlock()

		return fmt.Errorf("failed to consume messages: %w", err)
	}

	mq.subs[target] = subCh
	mq.mu.Unlock()

	go func() {
		for {
			select {
			case msg, ok := <-msgs:
				if !ok {
					return
				}

				spanCtx := propagation.TraceContext{}.Extract(ctx, amqpHeadersCarrier(msg.Headers))
				spanCtx, span := otel.Tracer("AMQP").Start(spanCtx, "ConsumeMessage")
				span.SetAttributes(attribute.String("queue", q.Name))

				message.Chan <- query.ResponseMessage{
					Body:    msg.Body,
					Context: spanCtx,
				}

				span.End()
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

func (mq *MQ) UnSubscribe(target string) error {
	queueName := fmt.Sprintf("%s-%s", target, mq.cfg.GetString("SERVICE_NAME"))

	mq.mu.Lock()

	subCh, ok := mq.subs[target]
	if !ok {
		mq.mu.Unlock()
		return nil
	}

	delete(mq.subs, target)
	mq.mu.Unlock()

	err := subCh.QueueUnbind(queueName, "", target, nil)
	if err != nil {
		_ = subCh.Close()

		return fmt.Errorf("failed to unbind queue: %w", err)
	}

	if err := subCh.Close(); err != nil {
		return fmt.Errorf("failed to close subscription channel: %w", err)
	}

	return nil
}
