/*
Config package
*/
package config

import (
	"errors"
	"sync"

	"github.com/spf13/viper"
)

type Config struct {
	mu sync.RWMutex
}

// New - read .env and ENV variables
func New() (*Config, error) {
	viper.SetConfigName(".env")
	viper.SetConfigType("dotenv")
	viper.AddConfigPath(".") // look for config in the working directory
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		var typeErr viper.ConfigFileNotFoundError
		if !errors.As(err, &typeErr) {
			return nil, err
		}
	}

	config := &Config{}

	// Enable feature toggle
	err := config.FeatureToogleRun()
	if err != nil {
		return nil, err
	}

	return config, nil
}
