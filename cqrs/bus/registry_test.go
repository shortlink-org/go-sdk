package bus

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
)

func TestTypeRegistryConcurrentAccess(t *testing.T) {
	reg := NewTypeRegistry()

	const workers = 50
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(2)

		go func(i int) {
			defer wg.Done()
			cmd := cqrsmessage.CommandEnvelope{
				Metadata: map[string]string{
					cqrsmessage.MetadataTypeName:    fmt.Sprintf("billing.command.create_%d", i),
					cqrsmessage.MetadataTypeVersion: "v1",
				},
			}
			err := reg.RegisterCommand(cmd)
			require.NoError(t, err, "register command %d", i)
		}(i)

		go func(i int) {
			defer wg.Done()
			evt := cqrsmessage.EventEnvelope{
				Metadata: map[string]string{
					cqrsmessage.MetadataTypeName:    fmt.Sprintf("billing.aggregate.event_%d", i),
					cqrsmessage.MetadataTypeVersion: "v1",
				},
			}
			err := reg.RegisterEvent(evt)
			require.NoError(t, err, "register event %d", i)
		}(i)
	}

	wg.Wait()

	for i := 0; i < workers; i++ {
		_, ok := reg.ResolveCommand(fmt.Sprintf("billing.command.create_%d.v1", i))
		require.True(t, ok, "command %d not found", i)

		_, ok = reg.ResolveEvent(fmt.Sprintf("billing.aggregate.event_%d.v1", i))
		require.True(t, ok, "event %d not found", i)
	}
}
