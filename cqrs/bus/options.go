package bus

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ThreeDotsLabs/watermill"
	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/otel/metric"
)

const defaultForwarderTopic = "shortlink_cqrs_outbox"

// Option configures Bus behaviours without breaking the constructor API.
type Option func(*cqrsConfig)

type cqrsConfig struct {
	outbox   *OutboxConfig
	txOutbox *txOutboxConfig
	err      error
}

// txOutboxConfig configures Publish to write to outbox using a transaction from context (go-sdk/uow).
type txOutboxConfig struct {
	ForwarderTopic string
	WMLogger       watermill.LoggerAdapter
}

// WithTxAwareOutbox makes Publish(ctx, evt) use the transaction from context when present (go-sdk/uow).
// When uow.HasTx(ctx) is true, the event is written to the outbox in the same transaction.
// ForwarderTopic and WMLogger are used to create a tx-scoped watermill-sql publisher wrapped with the forwarder.
func WithTxAwareOutbox(forwarderTopic string, wmLogger watermill.LoggerAdapter) Option {
	return func(c *cqrsConfig) {
		if c.err != nil {
			return
		}
		if forwarderTopic == "" {
			c.err = errors.New("cqrs/bus: forwarder topic is required for WithTxAwareOutbox")
			return
		}
		c.txOutbox = &txOutboxConfig{
			ForwarderTopic: forwarderTopic,
			WMLogger:       wmLogger,
		}
	}
}

// OutboxConfig wires transactional outbox pieces required by the Watermill forwarder.
type OutboxConfig struct {
	DB            *sql.DB
	Pool          *pgxpool.Pool
	Subscriber    wmmessage.Subscriber
	RealPublisher wmmessage.Publisher
	ForwarderName string
	Logger        logger.Logger
	MeterProvider metric.MeterProvider
}

// WithOutbox enables Watermill's Outbox/Forwarder transport.
//
// Example with *sql.DB:
//
//	package main
//
//	import (
//		"database/sql"
//
//		"github.com/ThreeDotsLabs/watermill-sql/v2/pkg/sql"
//		"github.com/shortlink-org/go-sdk/cqrs/bus"
//	)
//
//	func wireBuses(db *sql.DB, kafkaPub wmmessage.Publisher) *bus.CommandBus {
//		sqlPublisher, _ := sql.NewPublisher(db, sqlPublisherConfig, sqlLogger)
//		sqlSubscriber, _ := sql.NewSubscriber(db, sqlSubscriberConfig, sqlLogger)
//
//		return bus.NewCommandBus(
//			sqlPublisher,
//			marshaler,
//			namer,
//			bus.WithOutbox(bus.OutboxConfig{
//				DB:            db,
//				Subscriber:    sqlSubscriber,
//				RealPublisher: kafkaPub,
//				ForwarderName: "orders_outbox_forwarder",
//				Logger:        log,
//				MeterProvider: meterProvider,
//			}),
//		)
//	}
//
// Example with *pgxpool.Pool (converted automatically):
//
//	package main
//
//	import (
//		"database/sql"
//
//		"github.com/jackc/pgx/v5/pgxpool"
//		"github.com/ThreeDotsLabs/watermill-sql/v2/pkg/sql"
//		"github.com/shortlink-org/go-sdk/cqrs/bus"
//	)
//
//	func wireBusesWithPgx(pool *pgxpool.Pool, kafkaPub wmmessage.Publisher) *bus.CommandBus {
//		// Convert pgxpool to sql.DB for watermill-sql
//		sqlDB, _ := sql.Open("pgx", pool.Config().ConnString())
//		sqlPublisher, _ := sql.NewPublisher(sqlDB, sqlPublisherConfig, sqlLogger)
//		sqlSubscriber, _ := sql.NewSubscriber(sqlDB, sqlSubscriberConfig, sqlLogger)
//
//		return bus.NewCommandBus(
//			sqlPublisher,
//			marshaler,
//			namer,
//			bus.WithOutbox(bus.OutboxConfig{
//				Pool:          pool,
//				Subscriber:    sqlSubscriber,
//				RealPublisher: kafkaPub,
//				ForwarderName: "orders_outbox_forwarder",
//				Logger:        log,
//				MeterProvider: meterProvider,
//			}),
//		)
//	}
func WithOutbox(cfg OutboxConfig) Option {
	return func(c *cqrsConfig) {
		conf := cfg
		if c.err != nil {
			return
		}
		if err := conf.prepare(); err != nil {
			c.err = err
			return
		}
		c.outbox = &conf
	}
}

func applyOptions(opts []Option) (cqrsConfig, error) {
	var cfg cqrsConfig
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
		if cfg.err != nil {
			break
		}
	}
	return cfg, cfg.err
}

func (c *OutboxConfig) prepare() error {
	if c.DB == nil && c.Pool == nil {
		return errors.New("cqrs/bus: sql.DB or pgxpool.Pool must be provided")
	}
	if c.Subscriber == nil {
		return errors.New("cqrs/bus: outbox subscriber is required")
	}
	if c.RealPublisher == nil {
		return errors.New("cqrs/bus: real publisher is required")
	}
	if c.Logger == nil {
		return errors.New("cqrs/bus: logger is required")
	}
	if c.MeterProvider == nil {
		return errors.New("cqrs/bus: meter provider is required")
	}
	if c.DB == nil && c.Pool != nil {
		c.DB = stdlib.OpenDBFromPool(c.Pool)
	}
	c.ForwarderName = sanitizeForwarderTopic(c.ForwarderName, c.DB, c.Pool)
	return nil
}

func sanitizeForwarderTopic(name string, db *sql.DB, pool *pgxpool.Pool) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}

	driverName := "sql"
	if pool != nil {
		driverName = "pgxpool"
	} else if db != nil && db.Driver() != nil {
		driverName = fmt.Sprintf("%T", db.Driver())
	}

	driverName = strings.ToLower(driverName)
	driverName = strings.ReplaceAll(driverName, ".", "_")
	driverName = strings.ReplaceAll(driverName, "*", "_")
	driverName = strings.Trim(driverName, "_")
	if driverName == "" {
		driverName = "sql"
	}

	return fmt.Sprintf("%s_%s", defaultForwarderTopic, driverName)
}
