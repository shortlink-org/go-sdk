package kafka

import (
	"fmt"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/pkg/errors"

	"github.com/shortlink-org/go-sdk/config"
)

// Defaults and identifiers shared across the Kafka backend.
const (
	loggerName       = "watermill"
	defaultKafkaAddr = "localhost:9092"

	// Wait this long before asking the broker for metadata again.
	metadataRetryBackoff = 2 * time.Second

	// Presize a log-field map to the number of fields these packages set.
	logFieldCapacity = 4

	// Wait for a topic to exist for this long before giving up.
	defaultTopicWait = 10 * time.Second
)

type backendSettings struct {
	brokers                 []string
	consumerGroup           string
	enableOTEL              bool
	nackSleep               time.Duration
	reconnectSleep          time.Duration
	waitForTopicTimeout     time.Duration
	skipTopicInitialization bool

	publisherSarama  *sarama.Config
	subscriberSarama *sarama.Config
}

type kafkaConfig struct {
	brokers                 []string
	consumerGroup           string
	clientID                string
	enableOTEL              bool
	initialOffset           int64
	rebalanceStrategy       sarama.BalanceStrategy
	nackSleep               time.Duration
	reconnectSleep          time.Duration
	waitForTopicTimeout     time.Duration
	skipTopicInitialization bool
	version                 sarama.KafkaVersion
	producerRetryMax        int
	compression             sarama.CompressionCodec
	idempotentProducer      bool
}

func (s *backendSettings) publisherConfig() PublisherConfig {
	return PublisherConfig{
		Brokers:               s.brokers,
		OverwriteSaramaConfig: s.publisherSarama,
		OTELEnabled:           s.enableOTEL,
	}
}

func (s *backendSettings) subscriberConfig() SubscriberConfig {
	return SubscriberConfig{
		Brokers:                     s.brokers,
		ConsumerGroup:               s.consumerGroup,
		OverwriteSaramaConfig:       s.subscriberSarama,
		NackResendSleep:             s.nackSleep,
		ReconnectRetrySleep:         s.reconnectSleep,
		WaitForTopicCreationTimeout: s.waitForTopicTimeout,
		DoNotWaitForTopicCreation:   s.skipTopicInitialization,
		OTELEnabled:                 s.enableOTEL,
	}
}

func loadBackendSettings(cfg *config.Config) (*backendSettings, error) {
	kcfg, err := newKafkaConfig(cfg)
	if err != nil {
		return nil, err
	}

	pubSarama := DefaultSaramaSyncPublisherConfig()
	pubSarama.ClientID = kcfg.clientID
	pubSarama.Version = kcfg.version
	pubSarama.Producer.Retry.Max = kcfg.producerRetryMax
	pubSarama.Producer.RequiredAcks = sarama.WaitForAll
	pubSarama.Producer.Idempotent = kcfg.idempotentProducer

	pubSarama.Producer.Compression = kcfg.compression
	if kcfg.idempotentProducer {
		pubSarama.Net.MaxOpenRequests = 1
	}

	subSarama := DefaultSaramaSubscriberConfig()
	subSarama.ClientID = kcfg.clientID
	subSarama.Version = kcfg.version
	subSarama.Consumer.Offsets.Initial = kcfg.initialOffset
	subSarama.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{kcfg.rebalanceStrategy}

	return &backendSettings{
		brokers:                 kcfg.brokers,
		consumerGroup:           kcfg.consumerGroup,
		enableOTEL:              kcfg.enableOTEL,
		nackSleep:               kcfg.nackSleep,
		reconnectSleep:          kcfg.reconnectSleep,
		waitForTopicTimeout:     kcfg.waitForTopicTimeout,
		skipTopicInitialization: kcfg.skipTopicInitialization,
		publisherSarama:         pubSarama,
		subscriberSarama:        subSarama,
	}, nil
}

func newKafkaConfig(cfg *config.Config) (*kafkaConfig, error) {
	brokers := parseBrokerList(cfg)
	if len(brokers) == 0 {
		return nil, errors.New("WATERMILL_KAFKA_BROKERS must not be empty")
	}

	serviceName := strings.TrimSpace(cfg.GetString("SERVICE_NAME"))

	defaultGroup := serviceName
	if defaultGroup == "" {
		defaultGroup = loggerName
	}

	consumerGroup := firstNonEmpty(strings.TrimSpace(cfg.GetString("WATERMILL_KAFKA_CONSUMER_GROUP")), defaultGroup)
	if consumerGroup == "" {
		return nil, errors.New("WATERMILL_KAFKA_CONSUMER_GROUP must not be empty")
	}

	clientID := firstNonEmpty(strings.TrimSpace(cfg.GetString("WATERMILL_KAFKA_CLIENT_ID")), consumerGroup)

	initialOffset, err := parseInitialOffset(firstNonEmpty(cfg.GetString("WATERMILL_KAFKA_CONSUMER_INITIAL_OFFSET"), "latest"))
	if err != nil {
		return nil, err
	}

	strategy, err := parseRebalanceStrategy(firstNonEmpty(cfg.GetString("WATERMILL_KAFKA_REBALANCE_STRATEGY"), "range"))
	if err != nil {
		return nil, err
	}

	version, err := parseKafkaVersion(firstNonEmpty(cfg.GetString("WATERMILL_KAFKA_SARAMA_VERSION"), "max"))
	if err != nil {
		return nil, err
	}

	compression, err := parseCompressionCodec(firstNonEmpty(cfg.GetString("WATERMILL_KAFKA_PRODUCER_COMPRESSION"), "snappy"))
	if err != nil {
		return nil, err
	}

	producerRetryMax := cfg.GetInt("WATERMILL_KAFKA_PRODUCER_RETRY_MAX")
	if producerRetryMax == 0 {
		producerRetryMax = 10
	}

	enableOTEL := boolWithDefault(cfg, "WATERMILL_KAFKA_OTEL_ENABLED", true)
	idempotent := boolWithDefault(cfg, "WATERMILL_KAFKA_PRODUCER_IDEMPOTENT", true)
	nackSleep := durationWithDefault(cfg, "WATERMILL_KAFKA_SUBSCRIBER_NACK_SLEEP", 100*time.Millisecond)
	reconnectSleep := durationWithDefault(cfg, "WATERMILL_KAFKA_SUBSCRIBER_RECONNECT_SLEEP", time.Second)
	waitTimeout := durationWithDefault(cfg, "WATERMILL_KAFKA_WAIT_FOR_TOPIC_TIMEOUT", defaultTopicWait)
	skipTopicInit := boolWithDefault(cfg, "WATERMILL_KAFKA_SKIP_TOPIC_INIT", false)

	return &kafkaConfig{
		brokers:                 brokers,
		consumerGroup:           consumerGroup,
		clientID:                clientID,
		enableOTEL:              enableOTEL,
		initialOffset:           initialOffset,
		rebalanceStrategy:       strategy,
		nackSleep:               nackSleep,
		reconnectSleep:          reconnectSleep,
		waitForTopicTimeout:     waitTimeout,
		skipTopicInitialization: skipTopicInit,
		version:                 version,
		producerRetryMax:        producerRetryMax,
		compression:             compression,
		idempotentProducer:      idempotent,
	}, nil
}

func parseBrokerList(cfg *config.Config) []string {
	brokers := filterBrokers(cfg.GetStringSlice("WATERMILL_KAFKA_BROKERS"))
	if len(brokers) > 0 {
		return brokers
	}

	raw := cfg.GetString("WATERMILL_KAFKA_BROKERS")
	if raw == "" {
		return []string{defaultKafkaAddr}
	}

	parsed := filterBrokers(strings.Split(raw, ","))
	if len(parsed) == 0 {
		return []string{defaultKafkaAddr}
	}

	return parsed
}

func filterBrokers(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		result = append(result, value)
	}

	return result
}

// Configuration values a setting does not accept.
var (
	// ErrUnsupportedInitialOffset reports a consumer offset the parser does not know.
	ErrUnsupportedInitialOffset = errors.New("unsupported consumer initial offset")
	// ErrUnsupportedRebalanceStrategy reports an unrecognized rebalance strategy.
	ErrUnsupportedRebalanceStrategy = errors.New("unsupported rebalance strategy")
	// ErrUnsupportedCompression reports an unrecognized producer compression.
	ErrUnsupportedCompression = errors.New("unsupported producer compression")
)

func parseInitialOffset(raw string) (int64, error) {
	//nolint:ireturn // the interface is the library's own contract
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "latest", "newest":
		return sarama.OffsetNewest, nil
	case "oldest", "earliest":
		return sarama.OffsetOldest, nil
	default:
		return 0, fmt.Errorf("%w: WATERMILL_KAFKA_CONSUMER_INITIAL_OFFSET=%s", ErrUnsupportedInitialOffset, raw)
	}
}

//nolint:ireturn // sarama.BalanceStrategy is the library's own type
func parseRebalanceStrategy(raw string) (sarama.BalanceStrategy, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "range":
		return sarama.NewBalanceStrategyRange(), nil
	case "roundrobin", "round_robin":
		return sarama.NewBalanceStrategyRoundRobin(), nil
	case "sticky":
		return sarama.NewBalanceStrategySticky(), nil
	default:
		return nil, fmt.Errorf("%w: WATERMILL_KAFKA_REBALANCE_STRATEGY=%s", ErrUnsupportedRebalanceStrategy, raw)
	}
}

func parseKafkaVersion(raw string) (sarama.KafkaVersion, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "default":
		return sarama.DefaultVersion, nil
	case "max":
		return sarama.MaxVersion, nil
	default:
		version, err := sarama.ParseKafkaVersion(raw)
		if err != nil {
			return sarama.KafkaVersion{}, fmt.Errorf("invalid WATERMILL_KAFKA_SARAMA_VERSION: %w", err)
		}

		return version, nil
	}
}

func parseCompressionCodec(raw string) (sarama.CompressionCodec, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none":
		return sarama.CompressionNone, nil
	case "gzip":
		return sarama.CompressionGZIP, nil
	case "lz4":
		return sarama.CompressionLZ4, nil
	case "snappy":
		return sarama.CompressionSnappy, nil
	case "zstd":
		return sarama.CompressionZSTD, nil
	default:
		return sarama.CompressionNone, fmt.Errorf("%w: WATERMILL_KAFKA_PRODUCER_COMPRESSION=%s", ErrUnsupportedCompression, raw)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}

	return ""
}

func boolWithDefault(cfg *config.Config, key string, def bool) bool {
	if cfg.IsSet(key) {
		return cfg.GetBool(key)
	}

	return def
}

func durationWithDefault(cfg *config.Config, key string, def time.Duration) time.Duration {
	if cfg.IsSet(key) {
		if dur := cfg.GetDuration(key); dur != 0 {
			return dur
		}
	}

	return def
}
