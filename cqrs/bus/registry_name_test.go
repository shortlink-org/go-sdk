package bus

import (
	"testing"

	"github.com/stretchr/testify/require"

	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
)

type placeOrder struct{}

type orderPlaced struct{}

// The registry derives its key from NameOf, whose service segment comes from
// $SERVICE_NAME, while handlers look types up by the name the marshaler wrote on
// the message — and that service name comes from the namer the application
// built. Keying on the service segment made the two disagree for every service
// not literally named "shortlink", so nothing ever dispatched.
func TestResolveIgnoresTheServiceSegment(t *testing.T) {
	registry := NewTypeRegistry()
	require.NoError(t, registry.RegisterCommand(&placeOrder{}))
	require.NoError(t, registry.RegisterEvent(&orderPlaced{}))

	// A namer with a service name that has nothing to do with $SERVICE_NAME.
	namer := cqrsmessage.NewShortlinkNamer("billing")

	t.Run("command resolves under the namer's service", func(t *testing.T) {
		_, ok := registry.ResolveCommand(namer.CommandName(&placeOrder{}))
		require.True(t, ok)
	})

	t.Run("event resolves under the namer's service", func(t *testing.T) {
		_, ok := registry.ResolveEvent(namer.CommandName(&orderPlaced{}))
		require.True(t, ok)
	})

	t.Run("the default name still resolves", func(t *testing.T) {
		_, ok := registry.ResolveCommand(cqrsmessage.NameOf(&placeOrder{}))
		require.True(t, ok)
	})

	t.Run("an unrelated type does not resolve", func(t *testing.T) {
		_, ok := registry.ResolveCommand(namer.CommandName(&orderPlaced{}))
		require.False(t, ok)
	})
}

func TestRegisterRejectsNil(t *testing.T) {
	registry := NewTypeRegistry()

	require.ErrorIs(t, registry.RegisterCommand(nil), ErrNilCommandType)
	require.ErrorIs(t, registry.RegisterEvent(nil), ErrNilEventType)
}
