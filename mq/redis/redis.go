package redis

import (
	"context"
	"errors"
	"log/slog"

	"github.com/redis/rueidis"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db"
	dbredis "github.com/shortlink-org/go-sdk/db/drivers/redis"
	"github.com/shortlink-org/go-sdk/mq/query"
)

// ErrNotImplemented is returned by the Pub/Sub operations: the Redis backend only
// establishes the connection so far. Selecting MQ_TYPE=redis therefore fails on use
// instead of taking the process down.
var ErrNotImplemented = errors.New("redis: message queue is not implemented")

type Redis struct {
	client rueidis.Client //nolint:unused // TODO implement me
	cfg    *config.Config
}

func New(cfg *config.Config) *Redis {
	return &Redis{cfg: cfg}
}

func (r *Redis) Init(ctx context.Context, log *slog.Logger) error {
	mq := dbredis.New(trace.NewNoopTracerProvider(), metric.NewMeterProvider(), r.cfg)

	err := mq.Init(ctx)
	if err != nil {
		return err
	}

	r.client, err = db.Conn[rueidis.Client](mq)
	if err != nil {
		return err
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		errClose := r.close()
		if errClose != nil {
			log.Error("Redis close error",
				slog.String("error", errClose.Error()),
			)
		}
	}()

	return nil
}

// close - close connection
//
//nolint:unparam // ignore unused parameter
func (r *Redis) close() error {
	r.client.Close()

	return nil
}

func (r *Redis) Publish(_ context.Context, _ string, _, _ []byte) error {
	// TODO implement me
	return ErrNotImplemented
}

func (r *Redis) Subscribe(_ context.Context, _ string, _ query.Response) error {
	// TODO implement me
	return ErrNotImplemented
}

func (r *Redis) UnSubscribe(_ string) error {
	// TODO implement me
	return ErrNotImplemented
}
