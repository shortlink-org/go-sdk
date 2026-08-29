//go:build unit

package replica

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultOptionsAreValid(t *testing.T) {
	t.Parallel()

	require.NoError(t, DefaultOptions().Validate())
}

func TestOptionsValidateBoundaries(t *testing.T) {
	t.Parallel()

	options := DefaultOptions()
	options.PollInterval = 0
	options.PollJitter = 1
	options.MaxLagBytes = 0
	options.GateMaxWait = 0

	require.NoError(t, options.Validate())
}

func TestOptionsRejectInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		option string
		mutate func(*Options)
	}{
		{name: "negative poll interval", option: "PollInterval", mutate: func(o *Options) { o.PollInterval = -time.Nanosecond }},
		{name: "negative jitter", option: "PollJitter", mutate: func(o *Options) { o.PollJitter = -0.1 }},
		{name: "excessive jitter", option: "PollJitter", mutate: func(o *Options) { o.PollJitter = 1.1 }},
		{name: "not-a-number jitter", option: "PollJitter", mutate: func(o *Options) { o.PollJitter = math.NaN() }},
		{name: "infinite jitter", option: "PollJitter", mutate: func(o *Options) { o.PollJitter = math.Inf(1) }},
		{name: "zero probe timeout", option: "ProbeTimeout", mutate: func(o *Options) { o.ProbeTimeout = 0 }},
		{name: "zero stale threshold", option: "SampleStaleAfter", mutate: func(o *Options) { o.SampleStaleAfter = 0 }},
		{name: "negative lag budget", option: "MaxLagBytes", mutate: func(o *Options) { o.MaxLagBytes = -1 }},
		{name: "negative gate wait", option: "GateMaxWait", mutate: func(o *Options) { o.GateMaxWait = -time.Nanosecond }},
		{name: "unknown no-tracker policy", option: "NoTracker", mutate: func(o *Options) { o.NoTracker = NoTrackerPolicy(255) }},
		{name: "unknown fallback policy", option: "Fallback", mutate: func(o *Options) { o.Fallback = FallbackPolicy(255) }},
		{name: "unknown watermark policy", option: "Watermark", mutate: func(o *Options) { o.Watermark = WatermarkPolicy(255) }},
		{name: "nil classifier", option: "Classifier", mutate: func(o *Options) { o.Classifier = nil }},
		{name: "empty replica URI", option: "URIs", mutate: func(o *Options) { o.URIs = []string{"postgres://replica", "  "} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			options := DefaultOptions()
			tt.mutate(&options)

			err := options.Validate()
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidOptions)

			var optionErr *OptionError
			require.ErrorAs(t, err, &optionErr)
			assert.Equal(t, tt.option, optionErr.Option)
			assert.NotEmpty(t, optionErr.Constraint)
		})
	}
}

func TestOpenRejectsInvalidOptionsBeforeOpeningResources(t *testing.T) {
	t.Parallel()

	options := DefaultOptions()
	options.ProbeTimeout = 0

	_, err := Open(t.Context(), &Config{Options: options})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidOptions)

	var routerErr *Error
	require.ErrorAs(t, err, &routerErr)
	assert.Equal(t, opOpen, routerErr.Op)
}

func TestOpenRejectsNilConfig(t *testing.T) {
	t.Parallel()

	_, err := Open(t.Context(), nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidOptions)
}
