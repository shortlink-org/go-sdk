package config

import (
	"fmt"
	"time"

	"github.com/Unleash/unleash-go-sdk/v6"
	"github.com/spf13/viper"
)

const REFRESH_INTERVAL = 10 * time.Second

func (*Config) FeatureToogleRun() error {
	viper.SetDefault("FEATURE_TOGGLE_ENABLE", false)
	viper.SetDefault("FEATURE_TOGGLE_API", "http://localhost:4242/api/")

	isEnableFeatureToggle := viper.GetBool("FEATURE_TOGGLE_ENABLE")
	if !isEnableFeatureToggle {
		return nil
	}

	err := unleash.Initialize(
		unleash.WithListener(&unleash.DebugListener{}),
		unleash.WithAppName(viper.GetString("SERVICE_NAME")),
		unleash.WithUrl(viper.GetString("FEATURE_TOGGLE_API")),
		unleash.WithRefreshInterval(REFRESH_INTERVAL),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize feature toggle: %w", err)
	}

	return nil
}
