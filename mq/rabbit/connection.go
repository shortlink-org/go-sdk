package rabbit

import (
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Dial establishes the RabbitMQ connection with the automatic recovery built into
// amqp091-go: on a network failure the connection and all of its channels are
// reopened, the tracked topology (exchanges, queues, bindings) is re-declared and
// consumers are re-subscribed onto the delivery channels they already handed out.
//
// Recovery is opt-in on the library side — a nil Config.Recovery leaves the
// connection without any reconnection logic — and it stays disabled while
// MaxRetryCount is zero. Once the retries are exhausted the connection is closed
// for good and Check starts reporting the failure.
func (mq *MQ) Dial() error {
	conn, err := amqp.DialConfig(mq.config.URI, amqp.Config{
		Recovery: &amqp.Recovery{
			ReconnectionConfig: &amqp.ReconnectionConfig{
				MaxRetryCount: mq.config.MaxRetryCount,
				RetryInterval: mq.config.RetryInterval,
			},
			// TopologyRecoveryMode is left at its zero value,
			// TopologyRecoveryAllEnabled, so consumers are recovered too.
		},
	})
	if err != nil {
		return fmt.Errorf("rabbit dial: %w", err)
	}

	mq.conn = conn
	mq.state.Store(int32(amqp.StateOpen))
	mq.watchState(conn)

	return nil
}

// watchState mirrors the connection life cycle into mq.state so Check can tell a
// connection that is recovering apart from one that is gone, and logs every
// transition along the way.
func (mq *MQ) watchState(conn *amqp.Connection) {
	states := make(chan *amqp.StateChanged, 1)
	conn.NotifyStateChange(states)

	go func() {
		// The library closes the listener channel right after the terminal
		// StateClosed transition, which ends this goroutine.
		for state := range states {
			mq.state.Store(int32(state.To))

			fields := []slog.Attr{
				slog.String("from", state.From.String()),
				slog.String("to", state.To.String()),
			}

			switch {
			case state.Err != nil:
				mq.log.Error("RabbitMQ recovery failed", append(fields, slog.String("error", state.Err.Error()))...)
			case state.To == amqp.StateOpen && state.From == amqp.StateReconnecting:
				mq.log.Info("RabbitMQ connection recovered", fields...)
			case state.To == amqp.StateReconnecting:
				mq.log.Warn("RabbitMQ connection lost, recovering", fields...)
			default:
				mq.log.Info("RabbitMQ connection state changed", fields...)
			}

			for _, entity := range state.SkippedTopologyEntities {
				mq.log.Error("RabbitMQ topology entity was not recovered",
					slog.String("type", entity.EntityType.String()),
					slog.String("name", entity.EntityName),
					slog.String("error", entity.Err.Error()),
				)
			}
		}
	}()
}
