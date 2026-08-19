//go:build unit

package replica

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
