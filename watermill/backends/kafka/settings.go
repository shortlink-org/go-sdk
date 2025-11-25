package kafka

import (
	"fmt"
	"strings"
	"time"

	"github.com/IBM/sarama"

	"github.com/shortlink-org/go-sdk/config"
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
		return nil, fmt.Errorf("WATERMILL_KAFKA_BROKERS must not be empty")
	}

	serviceName := strings.TrimSpace(cfg.GetString("SERVICE_NAME"))
	defaultGroup := serviceName
	if defaultGroup == "" {
		defaultGroup = "watermill"
	}

	consumerGroup := firstNonEmpty(strings.TrimSpace(cfg.GetString("WATERMILL_KAFKA_CONSUMER_GROUP")), defaultGroup)
	if consumerGroup == "" {
		return nil, fmt.Errorf("WATERMILL_KAFKA_CONSUMER_GROUP must not be empty")
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
	waitTimeout := durationWithDefault(cfg, "WATERMILL_KAFKA_WAIT_FOR_TOPIC_TIMEOUT", 10*time.Second)
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
		return []string{"localhost:9092"}
	}

	parsed := filterBrokers(strings.Split(raw, ","))
	if len(parsed) == 0 {
		return []string{"localhost:9092"}
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

func parseInitialOffset(raw string) (int64, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "latest", "newest":
		return sarama.OffsetNewest, nil
	case "oldest", "earliest":
		return sarama.OffsetOldest, nil
	default:
		return 0, fmt.Errorf("unsupported WATERMILL_KAFKA_CONSUMER_INITIAL_OFFSET: %s", raw)
	}
}

func parseRebalanceStrategy(raw string) (sarama.BalanceStrategy, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "range":
		return sarama.NewBalanceStrategyRange(), nil
	case "roundrobin", "round_robin":
		return sarama.NewBalanceStrategyRoundRobin(), nil
	case "sticky":
		return sarama.NewBalanceStrategySticky(), nil
	default:
		return nil, fmt.Errorf("unsupported WATERMILL_KAFKA_REBALANCE_STRATEGY: %s", raw)
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
		return sarama.CompressionNone, fmt.Errorf("unsupported WATERMILL_KAFKA_PRODUCER_COMPRESSION: %s", raw)
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
