/*
Config package
*/
package config

import (
	"errors"

	"github.com/spf13/viper"
)

// Logger is our contract for the logger
type Logger interface {
	Warn(msg string, fields ...Fields)
}

// Init - read .env and ENV variables
func New(log Logger) (*Config, error) {
	viper.SetConfigName(".env")
	viper.SetConfigType("dotenv")
	viper.AddConfigPath(".") // look for config in the working directory
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		var typeErr viper.ConfigFileNotFoundError
		if !errors.As(err, &typeErr) {
			return nil, err
		}

		log.Warn("The .env file has not been found in the current directory")
	}

	config := &Config{}

	// Enable feature toggle
	err := config.FeatureToogleRun()
	if err != nil {
		return nil, err
	}

	return config, nil
}
