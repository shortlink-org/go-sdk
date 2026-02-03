package bus

import (
	"context"
	"testing"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/cqrs/message"
)

// goleakIgnoreOpts ignores known third-party goroutines that may still be
// running after integration tests (testcontainers reaper, pgx pool, sql.DB).
// Used by TestMain and by integration tests.
var goleakIgnoreOpts = []goleak.Option{
	goleak.IgnoreTopFunction("github.com/testcontainers/testcontainers-go.(*Reaper).connect.func1"),
	goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
	goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
}

// TestMain runs goleak.VerifyTestMain after all tests in the package,
// so any goroutine leak from any test is reported once at exit.
// See https://github.com/uber-go/goleak
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleakIgnoreOpts...)
}

// TestCommandBus_Send_NoGoroutineLeak ensures CommandBus with in-memory publisher
// does not leak goroutines after Send and normal usage.
func TestCommandBus_Send_NoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	pc := gochannel.NewGoChannel(gochannel.Config{}, watermill.NewStdLogger(false, false))
	namer := message.NewShortlinkNamer("leak-test")
	marshaler := message.NewJSONMarshaler(namer)

	cmdBus := NewCommandBus(pc, marshaler, namer)
	ctx := context.Background()

	err := cmdBus.Send(ctx, &struct {
		ID string `json:"id"`
	}{ID: "x"})
	require.NoError(t, err)
	_ = cmdBus
}

// TestEventBus_Publish_NoGoroutineLeak ensures EventBus with in-memory publisher
// does not leak goroutines after Publish.
func TestEventBus_Publish_NoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	pc := gochannel.NewGoChannel(gochannel.Config{}, watermill.NewStdLogger(false, false))
	namer := message.NewShortlinkNamer("leak-test")
	marshaler := message.NewJSONMarshaler(namer)

	evtBus := NewEventBus(pc, marshaler, namer)
	ctx := context.Background()

	err := evtBus.Publish(ctx, &struct {
		ID string `json:"id"`
	}{ID: "y"})
	require.NoError(t, err)
	_ = evtBus
}
