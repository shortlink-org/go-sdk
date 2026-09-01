// Package config provides thread-safe configuration management backed by Viper.
package config

import (
	"errors"
	"fmt"
	"sync"

	"github.com/spf13/viper"
)

// Config wraps Viper-backed settings with a mutex for safe concurrent reads.
type Config struct {
	mu sync.RWMutex

	// v is this Config's own Viper. The package-level Viper is a process-wide
	// singleton: sharing it would leave every Config guarding the same state
	// behind a different mutex, and let one Config's Set reach all the others.
	v *viper.Viper

	// featureToggle reports whether FeatureToogleRun initialized the Unleash
	// client, so Close knows whether there is anything to shut down.
	featureToggle bool
	closeOnce     sync.Once
}

// New - read .env and ENV variables.
func New() (*Config, error) {
	v := viper.New()
	v.SetConfigName(".env")
	v.SetConfigType("dotenv")
	v.AddConfigPath(".") // look for config in the working directory
	v.AutomaticEnv()

	err := v.ReadInConfig()
	if err != nil {
		// A missing .env is normal: the settings may come from the environment alone.
		//nolint:errcheck // AsType's first result is the matched error itself; only the match matters here
		if _, isMissing := errors.AsType[viper.ConfigFileNotFoundError](err); !isMissing {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	config := &Config{
		v: v,
	}

	// Enable feature toggle
	err = config.FeatureToogleRun()
	if err != nil {
		return nil, err
	}

	return config, nil
}
