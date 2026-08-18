package config

import (
	"fmt"
	"time"

	"github.com/Unleash/unleash-go-sdk/v6"
	"github.com/spf13/viper"
)

// REFRESH_INTERVAL controls how often the Unleash client refreshes feature toggles.
const REFRESH_INTERVAL = 10 * time.Second

// FeatureToogleRun initializes Unleash when feature toggles are enabled in configuration.
func (c *Config) FeatureToogleRun() error {
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

	c.mu.Lock()
	c.featureToggle = true
	c.mu.Unlock()

	return nil
}

// Close shuts the feature toggle client down, stopping its polling and metrics
// goroutines and flushing the usage metrics buffered since the last tick, which
// are otherwise dropped on every restart.
//
// Calling it is safe on a Config that never enabled feature toggles, and safe to
// repeat: unleash.Close closes its internal channels and would panic if it ran
// twice. Note that Unleash keeps a single process-wide default client, so this
// closes the client shared by every Config.
func (c *Config) Close() error {
	var err error

	c.closeOnce.Do(func() {
		c.mu.RLock()
		enabled := c.featureToggle
		c.mu.RUnlock()

		if !enabled {
			return
		}

		err = unleash.Close()
	})

	return err
}
