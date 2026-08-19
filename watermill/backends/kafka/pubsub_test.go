package kafka_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-kafka/v3/pkg/kafka"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/subscriber"
	"github.com/ThreeDotsLabs/watermill/pubsub/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kafkaBrokers() []string {
	brokers := os.Getenv("WATERMILL_TEST_KAFKA_BROKERS")
	if brokers != "" {
		return strings.Split(brokers, ",")
	}

	return []string{"localhost:9091", "localhost:9092", "localhost:9093"}
}

func newPubSub(
	t *testing.T,
	marshaler kafka.MarshalerUnmarshaler,
	consumerGroup string,
	saramaOpts ...func(*sarama.Config),
) (*kafka.Publisher, *kafka.Subscriber) {
	t.Helper()

	logger := watermill.NewStdLogger(false, false)

	var (
		err       error
		publisher *kafka.Publisher
	)

	retriesLeft := 5

	for {
		publisher, err = kafka.NewPublisher(kafka.PublisherConfig{
			Brokers:   kafkaBrokers(),
			Marshaler: marshaler,
		}, logger)
		if err == nil || retriesLeft == 0 {
			break
		}

		retriesLeft--
		t.Logf("cannot create kafka Publisher: %s, retrying (%d retries left)", err, retriesLeft)
		time.Sleep(time.Second * 2)
	}

	require.NoError(t, err)

	saramaConfig := kafka.DefaultSaramaSubscriberConfig()
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	saramaConfig.Admin.Timeout = time.Second * testTimeoutSec
	saramaConfig.Producer.RequiredAcks = sarama.WaitForAll
	saramaConfig.ChannelBufferSize = testMaxMessageKB
	saramaConfig.Consumer.Group.Heartbeat.Interval = time.Millisecond * testFlushMillis
	saramaConfig.Consumer.Group.Rebalance.Timeout = time.Second * 3

	for _, o := range saramaOpts {
		o(saramaConfig)
	}

	var sub *kafka.Subscriber

	retriesLeft = 5

	for {
		sub, err = kafka.NewSubscriber(
			kafka.SubscriberConfig{
				Brokers:               kafkaBrokers(),
				Unmarshaler:           marshaler,
				OverwriteSaramaConfig: saramaConfig,
				ConsumerGroup:         consumerGroup,
				InitializeTopicDetails: &sarama.TopicDetail{
					NumPartitions:     3,
					ReplicationFactor: 1,
				},
			},
			logger,
		)
		if err == nil || retriesLeft == 0 {
			break
		}

		retriesLeft--
		t.Logf("cannot create kafka Subscriber: %s, retrying (%d retries left)", err, retriesLeft)
		time.Sleep(time.Second * 2)
	}

	require.NoError(t, err)

	return publisher, sub
}

func generatePartitionKey(topic string, msg *message.Message) (string, error) {
	return msg.Metadata.Get("partition_key"), nil
}

//nolint:ireturn // the interface is the library's own contract
func createPubSubWithConsumerGroup(t *testing.T, consumerGroup string) (message.Publisher, message.Subscriber) {
	t.Helper()

	return newPubSub(t, kafka.DefaultMarshaler{}, consumerGroup)
}

//nolint:ireturn // the interface is the library's own contract
func createPubSub(t *testing.T) (message.Publisher, message.Subscriber) {
	t.Helper()

	return createPubSubWithConsumerGroup(t, "test")
}

//nolint:ireturn // the interface is the library's own contract
func createPartitionedPubSub(t *testing.T) (message.Publisher, message.Subscriber) {
	t.Helper()

	return newPubSub(t, kafka.NewWithPartitioningMarshaler(generatePartitionKey), "test")
}

//nolint:ireturn // the interface is the library's own contract
func createNoGroupPubSub(t *testing.T) (message.Publisher, message.Subscriber) {
	t.Helper()

	return newPubSub(t, kafka.DefaultMarshaler{}, "")
}

// Sizes and timeouts the pub/sub tests share.
const (
	testTimeoutSec   = 30
	testMaxMessageKB = 10240
	testFlushMillis  = 500
	testBatchSize    = 20
)

func TestPublishSubscribe(t *testing.T) {
	t.Helper()

	features := tests.Features{
		ConsumerGroups:      true,
		ExactlyOnceDelivery: false,
		GuaranteedOrder:     false,
		Persistent:          true,
	}

	tests.TestPubSub(
		t,
		features,
		createPubSub,
		createPubSubWithConsumerGroup,
	)
}

func TestPublishSubscribe_ordered(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping long tests")
	}

	t.Parallel()

	tests.TestPubSub(
		t,
		tests.Features{
			ConsumerGroups:      true,
			ExactlyOnceDelivery: false,
			GuaranteedOrder:     true,
			Persistent:          true,
		},
		createPartitionedPubSub,
		createPubSubWithConsumerGroup,
	)
}

func TestNoGroupSubscriber(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping long tests")
	}

	t.Parallel()

	tests.TestPubSub(
		t,
		tests.Features{
			ConsumerGroups:                   false,
			ExactlyOnceDelivery:              false,
			GuaranteedOrder:                  false,
			Persistent:                       true,
			NewSubscriberReceivesOldMessages: true,
		},
		createNoGroupPubSub,
		nil,
	)
}

func TestCtxValues(t *testing.T) {
	t.Helper()

	pub, sub := newPubSub(t, kafka.DefaultMarshaler{}, "")
	topicName := "topic_" + watermill.NewUUID()

	messagesToPublish := make([]*message.Message, 0, testBatchSize)

	for range testBatchSize {
		id := watermill.NewUUID()
		messagesToPublish = append(messagesToPublish, message.NewMessage(id, nil))
	}

	err := pub.Publish(topicName, messagesToPublish...)
	require.NoError(t, err, "cannot publish message")

	messages, err := sub.Subscribe(context.Background(), topicName)
	require.NoError(t, err)

	receivedMessages, all := subscriber.BulkReadWithDeduplication(messages, len(messagesToPublish), time.Second*10)
	require.True(t, all)

	expectedPartitionsOffsets := map[int32]int64{}

	for _, msg := range receivedMessages {
		partition, ok := kafka.MessagePartitionFromCtx(msg.Context())
		assert.True(t, ok)

		messagePartitionOffset, ok := kafka.MessagePartitionOffsetFromCtx(msg.Context())
		assert.True(t, ok)

		kafkaMsgTimestamp, ok := kafka.MessageTimestampFromCtx(msg.Context())
		assert.True(t, ok)
		assert.NotZero(t, kafkaMsgTimestamp)

		_, ok = kafka.MessageKeyFromCtx(msg.Context())
		assert.True(t, ok)

		if expectedPartitionsOffsets[partition] <= messagePartitionOffset {
			// kafka partition offset is offset of the last message + 1
			expectedPartitionsOffsets[partition] = messagePartitionOffset + 1
		}
	}

	assert.NotEmpty(t, expectedPartitionsOffsets)

	offsets, err := sub.PartitionOffset(topicName)
	require.NoError(t, err)
	assert.NotEmpty(t, offsets)

	assert.EqualValues(t, expectedPartitionsOffsets, offsets)

	require.NoError(t, pub.Close())
}

func TestPublishSubscribe_AutoCommitDisabled(t *testing.T) {
	t.Helper()

	t.Parallel()

	features := tests.Features{
		ConsumerGroups:      true,
		ExactlyOnceDelivery: false,
		GuaranteedOrder:     false,
		Persistent:          true,
		// Disabled AutoCommit slow down Pub/Sub because of making commits synchronously
		ForceShort: true,
	}

	pubSubConstructorWithConsumerGroup := func(t *testing.T, consumerGroup string) (message.Publisher, message.Subscriber) {
		t.Helper()

		return newPubSub(t, kafka.DefaultMarshaler{}, consumerGroup, func(config *sarama.Config) {
			// commit messages manually
			config.Consumer.Offsets.AutoCommit.Enable = false
		})
	}
	pubSubConstructor := func(t *testing.T) (message.Publisher, message.Subscriber) {
		t.Helper()

		return pubSubConstructorWithConsumerGroup(t, "test")
	}

	tests.TestPubSub(
		t,
		features,
		pubSubConstructor,
		pubSubConstructorWithConsumerGroup,
	)
}

//nolint:nonamedreturns // the name documents what the channel or error means here
func readAfterRetries(messagesCh <-chan *message.Message, retriesN int, timeout time.Duration) (receivedMessage *message.Message, ok bool) {
	retries := 0

MessagesLoop:
	for retries <= retriesN {
		select {
		case msg, ok := <-messagesCh:
			if !ok {
				break MessagesLoop
			}

			if retries > 0 {
				msg.Ack()
				return msg, true
			}

			msg.Nack()

			retries++
		case <-time.After(timeout):
			break MessagesLoop
		}
	}

	return nil, false
}

func TestCtxValuesAfterRetry(t *testing.T) {
	t.Helper()

	pub, sub := newPubSub(t, kafka.DefaultMarshaler{}, "")
	topicName := "topic_" + watermill.NewUUID()

	messagesToPublish := make([]*message.Message, 0, testBatchSize)

	id := watermill.NewUUID()
	messagesToPublish = append(messagesToPublish, message.NewMessage(id, nil))

	err := pub.Publish(topicName, messagesToPublish...)
	require.NoError(t, err, "cannot publish message")

	messages, err := sub.Subscribe(context.Background(), topicName)
	require.NoError(t, err)

	receivedMessage, ok := readAfterRetries(messages, 1, time.Second)
	assert.True(t, ok)

	expectedPartitionsOffsets := map[int32]int64{}
	partition, ok := kafka.MessagePartitionFromCtx(receivedMessage.Context())
	assert.True(t, ok)

	messagePartitionOffset, ok := kafka.MessagePartitionOffsetFromCtx(receivedMessage.Context())
	assert.True(t, ok)

	kafkaMsgTimestamp, ok := kafka.MessageTimestampFromCtx(receivedMessage.Context())
	assert.True(t, ok)
	assert.NotZero(t, kafkaMsgTimestamp)

	if expectedPartitionsOffsets[partition] <= messagePartitionOffset {
		// kafka partition offset is offset of the last message + 1
		expectedPartitionsOffsets[partition] = messagePartitionOffset + 1
	}

	assert.NotEmpty(t, expectedPartitionsOffsets)

	offsets, err := sub.PartitionOffset(topicName)
	require.NoError(t, err)
	assert.NotEmpty(t, offsets)

	assert.EqualValues(t, expectedPartitionsOffsets, offsets)

	require.NoError(t, pub.Close())
}
