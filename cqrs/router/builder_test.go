package router

import (
	"testing"

	"github.com/ThreeDotsLabs/watermill"
	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/stretchr/testify/require"
)

const (
	testTopic       = "billing.command.create"
	testService     = "billing"
	testHandlerName = "create"
)

func noopHandler(*wmmessage.Message) ([]*wmmessage.Message, error) {
	return nil, nil
}

// newPubSub returns an in-memory pub/sub usable as both subscriber and publisher.
func newPubSub(t *testing.T) *gochannel.GoChannel {
	t.Helper()

	pubsub := gochannel.NewGoChannel(gochannel.Config{}, watermill.NopLogger{})

	t.Cleanup(func() {
		require.NoError(t, pubsub.Close())
	})

	return pubsub
}

func validConfig() RouterConfig {
	return RouterConfig{
		ServiceName: testService,
		Handlers: []HandlerRegistration{
			{Name: testHandlerName, Topic: testTopic, Handler: noopHandler},
		},
	}
}

func TestNewRouterRejectsMissingDependencies(t *testing.T) {
	pubsub := newPubSub(t)

	tests := []struct {
		name       string
		logger     watermill.LoggerAdapter
		subscriber wmmessage.Subscriber
		publisher  wmmessage.Publisher
		cfg        RouterConfig
		wantErr    error
	}{
		{
			name:       "nil logger",
			logger:     nil,
			subscriber: pubsub,
			publisher:  pubsub,
			cfg:        validConfig(),
			wantErr:    errNilLogger,
		},
		{
			name:       "nil subscriber",
			logger:     watermill.NopLogger{},
			subscriber: nil,
			publisher:  pubsub,
			cfg:        validConfig(),
			wantErr:    errNilSubscriber,
		},
		{
			name:       "nil publisher",
			logger:     watermill.NopLogger{},
			subscriber: pubsub,
			publisher:  nil,
			cfg:        validConfig(),
			wantErr:    errNilPublisher,
		},
		{
			name:       "no handlers",
			logger:     watermill.NopLogger{},
			subscriber: pubsub,
			publisher:  pubsub,
			cfg:        RouterConfig{ServiceName: testService},
			wantErr:    errNoHandlers,
		},
		{
			name:       "handler func is nil",
			logger:     watermill.NopLogger{},
			subscriber: pubsub,
			publisher:  pubsub,
			cfg: RouterConfig{
				ServiceName: testService,
				Handlers:    []HandlerRegistration{{Name: testHandlerName, Topic: testTopic}},
			},
			wantErr: errNilHandlerLogic,
		},
		{
			name:       "topic is empty",
			logger:     watermill.NopLogger{},
			subscriber: pubsub,
			publisher:  pubsub,
			cfg: RouterConfig{
				ServiceName: testService,
				Handlers:    []HandlerRegistration{{Name: testHandlerName, Topic: "   ", Handler: noopHandler}},
			},
			wantErr: errEmptyTopic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, err := NewRouter(tt.logger, tt.subscriber, tt.publisher, tt.cfg)

			require.Nil(t, router)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// The nil-handler and empty-topic errors name the offending registration, so a
// caller with many handlers can tell which one is broken.
func TestNewRouterErrorsIdentifyTheHandler(t *testing.T) {
	pubsub := newPubSub(t)

	_, err := NewRouter(watermill.NopLogger{}, pubsub, pubsub, RouterConfig{
		ServiceName: testService,
		Handlers:    []HandlerRegistration{{Name: testHandlerName, Topic: testTopic}},
	})
	require.ErrorIs(t, err, errNilHandlerLogic)
	require.Contains(t, err.Error(), testTopic)

	_, err = NewRouter(watermill.NopLogger{}, pubsub, pubsub, RouterConfig{
		ServiceName: testService,
		Handlers:    []HandlerRegistration{{Name: testHandlerName, Topic: "", Handler: noopHandler}},
	})
	require.ErrorIs(t, err, errEmptyTopic)
	require.Contains(t, err.Error(), testHandlerName)
}

func TestNewRouterBuildsRouter(t *testing.T) {
	pubsub := newPubSub(t)

	router, err := NewRouter(watermill.NopLogger{}, pubsub, pubsub, RouterConfig{
		ServiceName: testService,
		Handlers: []HandlerRegistration{
			{Name: testHandlerName, Topic: testTopic, Handler: noopHandler},
			{Topic: "billing.event.created", Handler: noopHandler},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, router)

	handlerNames := make([]string, 0, 2)
	for name := range router.Handlers() {
		handlerNames = append(handlerNames, name)
	}

	// The second registration has no explicit name, so the builder derives one
	// from the service and the topic.
	require.ElementsMatch(t, []string{testHandlerName, "billing_billing.event.created_handler"}, handlerNames)
}

func TestSanitizeService(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty falls back", in: "", want: "cqrs"},
		{name: "blank falls back", in: "   ", want: "cqrs"},
		{name: "lowercased", in: "Billing", want: "billing"},
		{name: "spaces become underscores", in: "Order Service", want: "order_service"},
		{name: "trimmed", in: "  billing  ", want: "billing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, sanitizeService(tt.in))
		})
	}
}

func TestSanitizeTopic(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty falls back", in: "", want: "topic"},
		{name: "blank falls back", in: "  ", want: "topic"},
		{name: "lowercased", in: "Billing.Command", want: "billing.command"},
		{name: "wildcard is spelled out", in: "billing.*", want: "billing.wildcard"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, sanitizeTopic(tt.in))
		})
	}
}

func TestHandlerRegistrationSanitize(t *testing.T) {
	t.Run("keeps an explicit name", func(t *testing.T) {
		reg := HandlerRegistration{Name: testHandlerName, Topic: testTopic}.sanitize(testService)
		require.Equal(t, testHandlerName, reg.Name)
	})

	t.Run("derives a name from service and topic", func(t *testing.T) {
		reg := HandlerRegistration{Topic: "billing.*"}.sanitize(testService)
		require.Equal(t, "billing_billing.wildcard_handler", reg.Name)
	})

	t.Run("trims the topic", func(t *testing.T) {
		reg := HandlerRegistration{Name: testHandlerName, Topic: "  " + testTopic + " "}.sanitize(testService)
		require.Equal(t, testTopic, reg.Topic)
	})

	t.Run("a blank topic still yields a usable name", func(t *testing.T) {
		reg := HandlerRegistration{Topic: "  "}.sanitize(testService)
		require.Equal(t, "billing_topic_handler", reg.Name)
		require.Empty(t, reg.Topic)
	})
}

func TestEnumerateHandlersSanitizesEvery(t *testing.T) {
	regs := enumerateHandlers(RouterConfig{
		Handlers: []HandlerRegistration{
			{Topic: " a "},
			{Name: "explicit", Topic: "b"},
		},
	}, "svc")

	require.Len(t, regs, 2)
	require.Equal(t, "svc_a_handler", regs[0].Name)
	require.Equal(t, "a", regs[0].Topic)
	require.Equal(t, "explicit", regs[1].Name)
}

// Sanity check that the sentinels stay distinct, so errors.Is can tell the
// failure modes apart.
func TestErrorsAreDistinct(t *testing.T) {
	all := []error{
		errNilLogger, errNilSubscriber, errNilPublisher,
		errNoHandlers, errNilHandlerLogic, errEmptyTopic,
	}

	for i, a := range all {
		for j, b := range all {
			if i == j {
				continue
			}

			require.NotErrorIs(t, a, b)
		}
	}
}
