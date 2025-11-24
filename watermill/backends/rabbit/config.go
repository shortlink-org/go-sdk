package rabbit

import (
	"github.com/shortlink-org/go-sdk/config"
)

type Config struct {
	DSN string
}

func Load(cfg *config.Config) Config {
	cfg.SetDefault("MQ_RABBIT_DSN", "amqp://guest:guest@rabbitmq:5672/")
	return Config{
		DSN: cfg.GetString("MQ_RABBIT_DSN"),
	}
}
