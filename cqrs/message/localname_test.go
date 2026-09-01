package message

import "testing"

const localKey = "command.create_order.v1"

func TestLocalName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "drops the service segment",
			in:   "billing.command.create_order.v1",
			want: localKey,
		},
		{
			name: "a different service yields the same key",
			in:   "shortlink.command.create_order.v1",
			want: localKey,
		},
		{
			name: "already service-free is unchanged",
			in:   localKey,
			want: localKey,
		},
		{
			name: "short names are left alone",
			in:   "create_order",
			want: "create_order",
		},
		{
			name: "empty is left alone",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LocalName(tt.in)
			if got != tt.want {
				t.Errorf("LocalName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
