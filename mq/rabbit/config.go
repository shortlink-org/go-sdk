package rabbit

import (
	"github.com/shortlink-org/go-sdk/config"
)

type Config struct {
	URI           string
	ReconnectTime int
}

// setConfig - Construct a new RabbitMQ configuration.
func loadConfig(cfg *config.Config) *Config {
	cfg.SetDefault("MQ_RABBIT_URI", "amqp://localhost:5672") // RabbitMQ URI
	// RabbitMQ reconnects after delay seconds
	cfg.SetDefault("MQ_RECONNECT_DELAY_SECONDS", 3)

	return &Config{
		URI:           cfg.GetString("MQ_RABBIT_URI"),
		ReconnectTime: cfg.GetInt("MQ_RECONNECT_DELAY_SECONDS"),
	}
}
