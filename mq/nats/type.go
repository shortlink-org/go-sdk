package nats

import (
	"context"
	"net/url"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - configuration
type Config struct {
	URI         *url.URL
	ChannelSize int
}

type MQ struct {
	mu sync.Mutex

	client *nats.Conn
	config *Config

	subscribes map[string]*subscription
	cfg        *config.Config
}

// subscription - an active NATS subscription together with the goroutine draining it.
type subscription struct {
	sub    *nats.Subscription
	cancel context.CancelFunc

	// done is closed once the draining goroutine has returned.
	done chan struct{}
}

// stop - unsubscribe from the subject and wait for the draining goroutine to return.
//
// The message channel is deliberately left open: NATS may still be inside a delivery
// when Unsubscribe returns, and closing it would turn that into a send on a closed
// channel. An unreferenced channel is collected by the GC anyway.
func (s *subscription) stop() error {
	err := s.sub.Unsubscribe()

	s.cancel()
	<-s.done

	return err
}
