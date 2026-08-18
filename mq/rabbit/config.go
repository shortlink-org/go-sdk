package rabbit

import (
	"time"

	"github.com/shortlink-org/go-sdk/config"
)

type Config struct {
	URI string

	// RetryInterval is the delay between recovery attempts made by amqp091-go.
	RetryInterval time.Duration

	// MaxRetryCount is how many recovery attempts are made before the connection
	// is closed for good. Zero disables automatic recovery entirely.
	MaxRetryCount int
}

// loadConfig - Construct a new RabbitMQ configuration.
func loadConfig(cfg *config.Config) *Config {
	cfg.SetDefault("MQ_RABBIT_URI", "amqp://localhost:5672") // RabbitMQ URI
	// Delay between recovery attempts
	cfg.SetDefault("MQ_RECONNECT_DELAY_SECONDS", 3)
	// Recovery attempts before the connection is given up on. 0 disables recovery.
	cfg.SetDefault("MQ_RECONNECT_MAX_RETRIES", 60)

	return &Config{
		URI:           cfg.GetString("MQ_RABBIT_URI"),
		RetryInterval: time.Duration(cfg.GetInt("MQ_RECONNECT_DELAY_SECONDS")) * time.Second,
		MaxRetryCount: cfg.GetInt("MQ_RECONNECT_MAX_RETRIES"),
	}
}
