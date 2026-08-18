package config

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestConfigClose(t *testing.T) {
	t.Run("feature toggles disabled", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		cfg, err := New()
		require.NoError(t, err)

		require.NoError(t, cfg.Close())
		require.NoError(t, cfg.Close(), "Close stays a no-op when there is no client")
	})

	t.Run("feature toggles enabled", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if r.URL.Path == "/api/client/features" {
				_, _ = w.Write([]byte(`{"version":2,"features":[]}`))

				return
			}

			w.WriteHeader(http.StatusAccepted)
		}))
		t.Cleanup(srv.Close)

		viper.Set("SERVICE_NAME", "config-test")
		viper.Set("FEATURE_TOGGLE_ENABLE", true)
		viper.Set("FEATURE_TOGGLE_API", srv.URL+"/api/")

		cfg, err := New()
		require.NoError(t, err)

		// The second call must not reach unleash.Close again: it closes internal
		// channels and would panic on the already closed ones.
		require.NoError(t, cfg.Close())
		require.NoError(t, cfg.Close())
	})
}
