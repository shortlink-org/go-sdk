//go:build unit

package replica

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNonPositivePollIntervalDisablesThePoller(t *testing.T) {
	t.Parallel()

	for _, interval := range []time.Duration{0, -time.Second} {
		router := newTestRouter(t, 1, func(options *Options) {
			options.PollInterval = interval
		})

		router.gate.start(context.Background())
		assert.False(t, router.gate.started, "interval: %s", interval)
	}
}

func TestExcessiveJitterNeverProducesANonPositiveInterval(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1, func(options *Options) {
		options.PollInterval = time.Second
		options.PollJitter = 10
	})

	for range 100 {
		assert.Positive(t, router.gate.nextInterval())
	}
}

func TestIsZeroDelay(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value string
		want  bool
	}{
		"off, bare zero":        {value: "0", want: true},
		"off, with unit":        {value: "0ms", want: true},
		"off, seconds":          {value: "0s", want: true},
		"off, padded":           {value: "  0ms  ", want: true},
		"delayed, milliseconds": {value: "500ms", want: false},
		"delayed, minutes":      {value: "1min", want: false},
		"delayed, hours":        {value: "2h", want: false},
		"unreadable, empty":     {value: "", want: true},
		"unreadable, no number": {value: "off", want: true},
		"unreadable, overflow":  {value: "99999999999999999999ms", want: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, isZeroDelay(tt.value))
		})
	}
}
