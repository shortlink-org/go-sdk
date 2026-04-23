package rabbit

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

type MQ struct {
	mu sync.Mutex
	// subs holds one AMQP channel per subscribe target (exchange name).
	// Publish uses mq.ch; consume paths use dedicated channels to avoid concurrent use of one channel.
	subs map[string]*Channel

	config *Config
	cfg    *config.Config

	log  logger.Logger
	conn *Connection
	ch   *Channel
}

func New(log logger.Logger, cfg *config.Config) *MQ {
	return &MQ{
		log:    log,
		cfg:    cfg,
		config: loadConfig(cfg), // Set configuration
		subs:   make(map[string]*Channel),
	}
}

// Init initializes the RabbitMQ connection and sets up the channel.
// It also sets up a graceful shutdown mechanism to close the connection and channel
// when the context is done.
func (mq *MQ) Init(ctx context.Context, log logger.Logger) error {
	// connect to RabbitMQ server
	err := mq.Dial()
	if err != nil {
		return err
	}

	// create a channel
	mq.ch, err = mq.conn.Channel()
	if err != nil {
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
func (mq *MQ) close() error {
	var errs error

	mq.mu.Lock()
	for _, subCh := range mq.subs {
		err := subCh.Close()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	mq.subs = make(map[string]*Channel)
	mq.mu.Unlock()

	if mq.ch != nil {
		err := mq.ch.Close()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	if mq.conn != nil {
		err := mq.conn.Close()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

// Check verifies the connection status.
func (mq *MQ) Check(_ context.Context) error {
	if mq.conn.IsClosed() {
		return amqp.ErrClosed
	}

	return nil
}
