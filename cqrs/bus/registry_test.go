package bus

import (
	"fmt"
	"sync"
	"testing"

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
			if err := reg.RegisterCommand(cmd); err != nil {
				t.Errorf("register command %d: %v", i, err)
			}
		}(i)

		go func(i int) {
			defer wg.Done()
			evt := cqrsmessage.EventEnvelope{
				Metadata: map[string]string{
					cqrsmessage.MetadataTypeName:    fmt.Sprintf("billing.aggregate.event_%d", i),
					cqrsmessage.MetadataTypeVersion: "v1",
				},
			}
			if err := reg.RegisterEvent(evt); err != nil {
				t.Errorf("register event %d: %v", i, err)
			}
		}(i)
	}

	wg.Wait()

	for i := 0; i < workers; i++ {
		if _, ok := reg.ResolveCommand(fmt.Sprintf("billing.command.create_%d.v1", i)); !ok {
			t.Fatalf("command %d not found", i)
		}
		if _, ok := reg.ResolveEvent(fmt.Sprintf("billing.aggregate.event_%d.v1", i)); !ok {
			t.Fatalf("event %d not found", i)
		}
	}
}
