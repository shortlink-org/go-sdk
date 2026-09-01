package rabbit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/shortlink-org/go-sdk/config"
)

type MQ struct {
	mu sync.Mutex
	// subs holds one AMQP channel per subscribe target (exchange name).
	// Publish uses mq.ch; consume paths use dedicated channels to avoid concurrent use of one channel.
	subs map[string]*amqp.Channel

	config *Config
	cfg    *config.Config

	log  *slog.Logger
	conn *amqp.Connection
	ch   *amqp.Channel

	// state mirrors the connection life cycle reported by the amqp091-go recovery.
	state atomic.Int32
}

func New(log *slog.Logger, cfg *config.Config) *MQ {
	mq := &MQ{
		log:    log,
		cfg:    cfg,
		config: loadConfig(cfg), // Set configuration
		subs:   make(map[string]*amqp.Channel),
	}

	mq.state.Store(int32(amqp.StateClosed))

	return mq
}

// Init initializes the RabbitMQ connection and sets up the channel.
// It also sets up a graceful shutdown mechanism to close the connection and channel
// when the context is done.
func (mq *MQ) Init(ctx context.Context, log *slog.Logger) error {
	// connect to RabbitMQ server
	err := mq.Dial()
	if err != nil {
		return err
	}

	// create a channel
	mq.ch, err = mq.conn.Channel()
	if err != nil {
		// Drop the connection so its recovery machinery does not outlive the failed Init.
		_ = mq.conn.Close()

		return err
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		errClose := mq.close()
		if errClose != nil {
			log.Error("RabbitMQ close error",
				slog.String("error", errClose.Error()),
			)
		}
	}()

	return nil
}

// close gracefully closes subscription channels, the publish channel, and the connection.
// Entities the broker already tore down report amqp.ErrClosed, which is not a shutdown failure.
func (mq *MQ) close() error {
	var errs error

	closeEntity := func(closer func() error) {
		err := closer()
		if err != nil && !errors.Is(err, amqp.ErrClosed) {
			errs = errors.Join(errs, err)
		}
	}

	mq.mu.Lock()
	for _, subCh := range mq.subs {
		closeEntity(subCh.Close)
	}

	mq.subs = make(map[string]*amqp.Channel)
	mq.mu.Unlock()

	if mq.ch != nil {
		closeEntity(mq.ch.Close)
	}

	if mq.conn != nil {
		closeEntity(mq.conn.Close)
	}

	return errs
}

// Check verifies the connection status. A connection that is recovering is reported
// as unhealthy: publishing fails until the recovery completes.
func (mq *MQ) Check(_ context.Context) error {
	if mq.conn == nil {
		return amqp.ErrClosed
	}

	state := amqp.LifeCycleState(mq.state.Load())
	if state != amqp.StateOpen {
		return fmt.Errorf("rabbit connection is %s: %w", state, amqp.ErrClosed)
	}

	return nil
}
