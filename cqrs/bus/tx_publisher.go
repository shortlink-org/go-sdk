package bus

import (
	"github.com/ThreeDotsLabs/watermill"
	wmsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/components/forwarder"
	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"github.com/jackc/pgx/v5"
)

// newTxPublisher creates a watermill publisher that writes to the outbox table in the same transaction as tx.
func newTxPublisher(tx pgx.Tx, cfg *txOutboxConfig) (wmmessage.Publisher, error) {
	if cfg == nil {
		return nil, nil
	}
	wmLogger := cfg.WMLogger
	if wmLogger == nil {
		wmLogger = watermill.NewStdLogger(false, false)
	}
	sqlTx := wmsql.TxFromPgx(tx)
	sqlPub, err := wmsql.NewPublisher(
		sqlTx,
		wmsql.PublisherConfig{
			SchemaAdapter:        wmsql.DefaultPostgreSQLSchema{},
			AutoInitializeSchema: false,
		},
		wmLogger,
	)
	if err != nil {
		return nil, err
	}
	return forwarder.NewPublisher(sqlPub, forwarder.PublisherConfig{
		ForwarderTopic: cfg.ForwarderTopic,
	}), nil
}
