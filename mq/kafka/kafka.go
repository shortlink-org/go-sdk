package kafka

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/dnwe/otelsarama"
	"github.com/heptiolabs/healthcheck"
	"go.opentelemetry.io/otel"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/mq/query"
)

// CooperativeStickyMigrationStrategyName is the intermediate step of the two-phase
// rollout to cooperative-sticky. Members advertise cooperative-sticky first and every
// eager strategy after it, so a group whose members are not upgraded yet keeps using
// its current eager strategy, and the group switches to cooperative-sticky by itself
// once the last eager member is gone. A group cannot mix eager and cooperative members,
// so existing groups must pass through this value before moving to cooperative-sticky.
const CooperativeStickyMigrationStrategyName = "cooperative-sticky-migration"

const (
	// healthcheckPeriod - how often the broker connection is re-checked.
	healthcheckPeriod = 5 * time.Second

	// minConsumeBackoff, maxConsumeBackoff - bounds of the backoff applied between
	// failed `Consume` calls.
	minConsumeBackoff = 100 * time.Millisecond
	maxConsumeBackoff = 30 * time.Second
)

// ErrSubscribeStopped is returned when the consume loop stops before the
// subscription becomes ready, e.g. because the context was canceled.
var ErrSubscribeStopped = errors.New("kafka: consumer stopped before the subscription was ready")

type Config struct {
	ConsumerGroup string
	URI           []string
}

type Kafka struct {
	*Config

	cfg *config.Config
	log logger.Logger

	client   sarama.Client
	producer sarama.SyncProducer

	// Use a sync.Map to keep track of the active subscriptions
	subscriptions sync.Map
}

// subscription holds everything needed to stop consuming a single target.
type subscription struct {
	cancel context.CancelFunc
	group  sarama.ConsumerGroup

	// done is closed once the consume loop has returned.
	done chan struct{}
}

// stop cancels the consume loop, waits for it to return and leaves the group.
func (s *subscription) stop() error {
	s.cancel()
	<-s.done

	return s.group.Close()
}

// New creates a Kafka MQ instance configured by cfg.
func New(cfg *config.Config) *Kafka {
	return &Kafka{
		Config: &Config{},
		cfg:    cfg,
	}
}

func (mq *Kafka) Init(ctx context.Context, log logger.Logger) error {
	mq.log = log

	// Set configuration
	config, err := mq.setConfig()
	if err != nil {
		return err
	}

	if mq.client, err = sarama.NewClient(mq.URI, config); err != nil {
		return err
	}

	// Create new producer
	producer, err := sarama.NewSyncProducerFromClient(mq.client)
	if err != nil {
		return errors.Join(err, mq.close())
	}

	// OpenTelemetry
	mq.producer = otelsarama.WrapSyncProducer(config, producer)

	// Check connection
	if len(mq.client.Brokers()) == 0 {
		return errors.Join(sarama.ErrOutOfBrokers, mq.close())
	}

	// run cron for check connection
	healthcheck.AsyncWithContext(ctx, func() error {
		if len(mq.client.Brokers()) > 0 {
			return nil
		}

		return sarama.ErrOutOfBrokers
	}, healthcheckPeriod)

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		errClose := mq.close()
		if errClose != nil {
			log.Error("Kafka close error",
				slog.Any("error", errClose),
			)
		}
	}()

	return nil
}

// close - Close all connections
func (mq *Kafka) close() error {
	var errs error

	// The consumer groups and the producer are built on top of the client,
	// so they have to be released before it.
	mq.subscriptions.Range(func(key, value any) bool {
		sub, ok := value.(*subscription)
		if !ok {
			return true
		}

		mq.subscriptions.Delete(key)
		errs = errors.Join(errs, sub.stop())

		return true
	})

	if mq.producer != nil {
		errs = errors.Join(errs, mq.producer.Close())
	}

	if mq.client != nil && !mq.client.Closed() {
		errs = errors.Join(errs, mq.client.Close())
	}

	return errs
}

func (mq *Kafka) Publish(ctx context.Context, target string, routingKey, payload []byte) error {
	msg := &sarama.ProducerMessage{
		Topic:     target,
		Key:       sarama.StringEncoder(routingKey),
		Value:     sarama.ByteEncoder(payload),
		Headers:   nil,
		Metadata:  nil,
		Offset:    0,
		Partition: 0,
	}

	// Carry the trace context in the message headers: this is the only channel
	// otelsarama has to link the producer span with the consumer one.
	otel.GetTextMapPropagator().Inject(ctx, otelsarama.NewProducerMessageCarrier(msg))

	_, _, err := mq.producer.SendMessage(msg)

	return err
}

// Subscribe - subscribe to message
func (mq *Kafka) Subscribe(ctx context.Context, target string, message query.Response) error {
	// Every target gets its own consumer group instance: a single one cannot run
	// concurrent `Consume` calls, and sharing it would tie the targets together.
	group, err := sarama.NewConsumerGroupFromClient(mq.ConsumerGroup, mq.client)
	if err != nil {
		return err
	}

	// Set up a new Sarama consumer group
	consumer := newConsumer(message)

	// OpenTelemetry
	handler := otelsarama.WrapConsumerGroupHandler(consumer)

	subCtx, cancel := context.WithCancel(ctx)

	sub := &subscription{
		cancel: cancel,
		group:  group,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(sub.done)

		mq.consume(subCtx, group, target, handler)
	}()

	// Wait until the consumer has been set up, or until the loop gives up
	select {
	case <-consumer.ready:
	case <-sub.done:
		cancel()

		return errors.Join(ErrSubscribeStopped, subCtx.Err(), group.Close())
	}

	// Keep track of subscriptions to be able to stop them
	mq.subscriptions.Store(target, sub)

	return nil
}

// consume - run the consume loop until the context is canceled or the group is closed.
func (mq *Kafka) consume(ctx context.Context, group sarama.ConsumerGroup, target string, handler sarama.ConsumerGroupHandler) {
	backoff := minConsumeBackoff

	for {
		// `Consume` should be called inside an infinite loop, when a
		// server-side rebalance happens, the consumer session will need to be
		// recreated to get the new claims
		err := group.Consume(ctx, []string{target}, handler)

		switch {
		case err == nil:
			backoff = minConsumeBackoff
		case errors.Is(err, sarama.ErrClosedConsumerGroup), errors.Is(err, context.Canceled):
			return
		default:
			// A failed `Consume` is usually transient (coordinator moved, connection
			// dropped): report it and retry instead of taking the process down.
			mq.log.ErrorWithContext(ctx, "Kafka consume error",
				slog.String("topic", target),
				slog.Any("error", err),
			)

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}

			backoff = min(backoff*2, maxConsumeBackoff)
		}

		// check if context was canceled, signaling that the consumer should stop
		if ctx.Err() != nil {
			return
		}
	}
}

func (mq *Kafka) UnSubscribe(target string) error {
	value, ok := mq.subscriptions.LoadAndDelete(target)
	if !ok {
		return nil
	}

	sub, okType := value.(*subscription)
	if !okType {
		return nil
	}

	return sub.stop()
}

// setConfig - Construct a new Sarama configuration.
//
// Reference:
// - https://developers.redhat.com/articles/2022/05/03/fine-tune-kafka-performance-kafka-optimization-theorem#the_kafka_optimization_theorem
func (mq *Kafka) setConfig() (*sarama.Config, error) {
	mq.cfg.SetDefault("MQ_KAFKA_URI", "localhost:9092")                                                         // Kafka URI
	mq.cfg.SetDefault("MQ_KAFKA_CONSUMER_GROUP", mq.cfg.GetString("SERVICE_NAME"))                              // Kafka consumer group
	mq.cfg.SetDefault("MQ_KAFKA_CONSUMER_GROUP_PARTITION_ASSIGNMENT_STRATEGY", sarama.RangeBalanceStrategyName) // Consumer group partition assignment strategy (range, roundrobin, sticky, cooperative-sticky, cooperative-sticky-migration)
	mq.cfg.SetDefault("MQ_KAFKA_CONSUMER_GROUP_OFFSET", sarama.OffsetNewest)                                    // Kafka consumer consumes initial offset from oldest
	mq.cfg.SetDefault("MQ_KAFKA_PRODUCER_RETRY_MAX", 3)                                                         // Kafka producer retry max
	mq.cfg.SetDefault("MQ_KAFKA_SARAMA_VERSION", "MAX")                                                         // Kafka sarama version: MAX, DEFAULT

	mq.Config = &Config{
		URI: []string{
			mq.cfg.GetString("MQ_KAFKA_URI"),
		},
		ConsumerGroup: mq.cfg.GetString("MQ_KAFKA_CONSUMER_GROUP"),
	}

	// sarama config
	saramaConfig := sarama.NewConfig()
	saramaConfig.ClientID = mq.cfg.GetString("SERVICE_NAME")

	strategy := mq.cfg.GetString("MQ_KAFKA_CONSUMER_GROUP_PARTITION_ASSIGNMENT_STRATEGY")
	switch strategy {
	case sarama.StickyBalanceStrategyName:
		saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategySticky()}
	case sarama.CooperativeStickyBalanceStrategyName:
		// Incremental rebalance: partitions are not revoked from every consumer at once.
		// The strategy is stateful and requires Version >= V2_4_0_0.
		saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyCooperativeSticky()}
	case CooperativeStickyMigrationStrategyName:
		saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
			sarama.NewBalanceStrategyCooperativeSticky(),
			sarama.NewBalanceStrategySticky(),
			sarama.NewBalanceStrategyRoundRobin(),
			sarama.NewBalanceStrategyRange(),
		}
	case sarama.RoundRobinBalanceStrategyName:
		saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	case sarama.RangeBalanceStrategyName:
		saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRange()}
	default:
		return nil, sarama.ErrConsumerCoordinatorNotAvailable
	}

	saramaConfig.Consumer.Offsets.Initial = mq.cfg.GetInt64("MQ_KAFKA_CONSUMER_GROUP_OFFSET")

	saramaConfig.Producer.Partitioner = sarama.NewRandomPartitioner
	saramaConfig.Producer.RequiredAcks = sarama.WaitForAll
	saramaConfig.Producer.Retry.Max = mq.cfg.GetInt("MQ_KAFKA_PRODUCER_RETRY_MAX")
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Consumer.Return.Errors = true
	saramaConfig.Producer.Compression = sarama.CompressionSnappy

	// set sarama version for support redpanda
	switch mq.cfg.GetString("MQ_KAFKA_SARAMA_VERSION") {
	case "MAX":
		saramaConfig.Version = sarama.MaxVersion
	case "DEFAULT":
		saramaConfig.Version = sarama.DefaultVersion
	}

	// idempotent producer
	saramaConfig.Producer.Idempotent = true
	if saramaConfig.Producer.Idempotent {
		if saramaConfig.Producer.Retry.Max == 0 {
			return nil, sarama.ErrInvalidConfig
		}

		if saramaConfig.Producer.RequiredAcks != sarama.WaitForAll {
			return nil, sarama.ErrInvalidConfig
		}

		saramaConfig.Net.MaxOpenRequests = 1
	}

	return saramaConfig, nil
}
