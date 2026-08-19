//go:build unit

package wal

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Values reused across the LSN tables.
const (
	sampleLSN      = LSN(0x16<<halfBits | 0xB374D848)
	sampleFlat     = LSN(0x16B374D848)
	sampleSystemID = uint64(7482913740192837465)
	sampleMillis   = int64(1755561600123)
	sampleTimeline = uint32(3)
)

func TestParseLSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    LSN
		wantErr bool
	}{
		{name: "typical", in: "16/B374D848", want: sampleLSN},
		{name: "lowercase", in: "16/b374d848", want: sampleLSN},
		{name: "zero", in: "0/0", want: 0},
		{name: "high half only", in: "1/0", want: LSN(1 << 32)},
		{name: "max", in: "FFFFFFFF/FFFFFFFF", want: Unknown},
		{name: "padded", in: "00000016/B374D848", want: sampleLSN},

		// PostgreSQL returns NULL, not a string, from pg_last_wal_replay_lsn()
		// on a promoted node. The caller must handle NULL before reaching here;
		// an empty string must not parse to a valid position.
		{name: "empty", in: "", wantErr: true},
		{name: "null literal", in: "NULL", wantErr: true},
		{name: "no slash", in: "16B374D848", wantErr: true},
		{name: "leading slash", in: "/B374D848", wantErr: true},
		{name: "trailing slash", in: "16/", wantErr: true},
		{name: "not hex", in: "zz/B374D848", wantErr: true},
		{name: "overflows high half", in: "1FFFFFFFF/0", wantErr: true},
		{name: "overflows low half", in: "0/1FFFFFFFF", wantErr: true},
		{name: "signed", in: "+16/B374D848", wantErr: true},
		{name: "hex prefix", in: "0x16/0xB374D848", wantErr: true},
		{name: "three parts", in: "16/B374/D848", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseLSN(tt.in)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidLSN)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLSNString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   LSN
		want string
	}{
		{name: "typical", in: sampleLSN, want: "16/B374D848"},
		{name: "zero", in: 0, want: "0/0"},
		{name: "unknown", in: Unknown, want: "FFFFFFFF/FFFFFFFF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.in.String())
		})
	}
}

func TestLSNRoundTrip(t *testing.T) {
	t.Parallel()

	for _, want := range []LSN{0, 1, math.MaxUint32, math.MaxUint32 + 1, sampleFlat, Unknown} {
		got, err := ParseLSN(want.String())
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

// TestUnknownIsMaximal guards the property the whole degradation path rests
// on: a write whose position could not be determined must never look replayed.
func TestUnknownIsMaximal(t *testing.T) {
	t.Parallel()

	for _, replayed := range []LSN{0, 1, math.MaxUint32, sampleFlat, Unknown - 1} {
		assert.Less(t, replayed, Unknown)
	}
}

func TestTokenRoundTrip(t *testing.T) {
	t.Parallel()

	want := Token{
		SystemID: sampleSystemID,
		Timeline: sampleTimeline,
		LSN:      sampleLSN,
		IssuedAt: time.UnixMilli(sampleMillis),
	}

	got, err := ParseToken(want.String())
	require.NoError(t, err)
	assert.Equal(t, want.SystemID, got.SystemID)
	assert.Equal(t, want.Timeline, got.Timeline)
	assert.Equal(t, want.LSN, got.LSN)
	assert.True(t, want.IssuedAt.Equal(got.IssuedAt))
}

func TestTokenStringShape(t *testing.T) {
	t.Parallel()

	token := Token{
		SystemID: sampleSystemID,
		Timeline: sampleTimeline,
		LSN:      sampleLSN,
		IssuedAt: time.UnixMilli(sampleMillis),
	}

	assert.Equal(t, fmt.Sprintf("v1:%d:%d:16/B374D848:%d", sampleSystemID, sampleTimeline, sampleMillis), token.String())
}

func TestParseToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "wrong version", in: "v2:42:3:16/B374D848:1755561600123"},
		{name: "no version", in: "42:3:16/B374D848:1755561600123"},
		{name: "too few fields", in: "v1:42:3:16/B374D848"},
		{name: "too many fields", in: "v1:42:3:16/B374D848:1755561600123:extra"},
		{name: "system id not a number", in: "v1:abc:3:16/B374D848:1755561600123"},
		{name: "timeline overflows uint32", in: "v1:42:4294967296:16/B374D848:1755561600123"},
		{name: "bad lsn", in: "v1:42:3:zz:1755561600123"},
		{name: "bad timestamp", in: "v1:42:3:16/B374D848:later"},
		{name: "oversized", in: fmt.Sprintf("v1:%d:%d:16/B374D848:%d", sampleSystemID, sampleTimeline, sampleMillis) + string(make([]byte, maxTokenLen))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseToken(tt.in)
			require.ErrorIs(t, err, ErrInvalidToken)
		})
	}
}

// TestTokenSameLineage covers the failover case: an LSN from another timeline
// names a WAL position that may never have existed on the current primary, so
// the two are not comparable at all.
func TestTokenSameLineage(t *testing.T) {
	t.Parallel()

	base := Token{SystemID: sampleSystemID, Timeline: sampleTimeline, LSN: 100}

	assert.True(t, base.SameLineage(Token{SystemID: sampleSystemID, Timeline: sampleTimeline, LSN: 999}))
	assert.False(t, base.SameLineage(Token{SystemID: sampleSystemID, Timeline: 4, LSN: 100}), "timeline switch")
	assert.False(t, base.SameLineage(Token{SystemID: 43, Timeline: sampleTimeline, LSN: 100}), "different cluster")
}

func TestTokenIsZero(t *testing.T) {
	t.Parallel()

	assert.True(t, Token{}.IsZero())
	assert.False(t, Token{LSN: 1}.IsZero())
	assert.False(t, Token{Timeline: 1}.IsZero())
	assert.False(t, Token{SystemID: 1}.IsZero())
}
