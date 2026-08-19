package config

import (
	"time"

	"github.com/spf13/viper"
)

// ----------------- Setters (write-locked) -----------------

// SetDefault sets a default value for a key.
// This value will be used if no other source provides a value.
func (c *Config) SetDefault(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.v.SetDefault(key, value)
}

// Set explicitly sets a value for a key at runtime.
func (c *Config) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.v.Set(key, value)
}

// AutomaticEnv enables automatic environment variable bindings.
func (c *Config) AutomaticEnv() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.v.AutomaticEnv()
}

// Reset clears all configuration values, this Config's only. Viper offers no
// per-instance reset, so the instance is replaced by an empty one — which also
// drops the config file and environment binding New set up, matching what the
// package-level Reset used to do.
func (c *Config) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.v = viper.New()
}

// ----------------- Getters (read-locked) ------------------

// GetString returns the value associated with the key as a string.
func (c *Config) GetString(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.GetString(key)
}

// GetBool returns the value associated with the key as a boolean.
func (c *Config) GetBool(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.GetBool(key)
}

// GetInt returns the value associated with the key as an int.
func (c *Config) GetInt(key string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.GetInt(key)
}

// GetInt64 returns the value associated with the key as an int64.
func (c *Config) GetInt64(key string) int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.GetInt64(key)
}

// GetUint64 returns the value associated with the key as a uint64.
func (c *Config) GetUint64(key string) uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.GetUint64(key)
}

// GetFloat64 returns the value associated with the key as a float64.
func (c *Config) GetFloat64(key string) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.GetFloat64(key)
}

// GetDuration returns the value associated with the key as a time.Duration.
func (c *Config) GetDuration(key string) time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.GetDuration(key)
}

// GetStringSlice returns the value associated with the key as a slice of strings.
func (c *Config) GetStringSlice(key string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.GetStringSlice(key)
}

// GetTime returns the value associated with the key as a time.Time.
func (c *Config) GetTime(key string) time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.GetTime(key)
}

// ----------------- Additional helpers -----------------

// IsSet checks whether the key exists in any configuration source.
func (c *Config) IsSet(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.IsSet(key)
}

// AllKeys returns all keys known to Viper across all configuration sources.
func (c *Config) AllKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.v.AllKeys()
}
