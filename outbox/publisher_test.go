//go:build unit || (database && postgres)

package outbox_test

import (
	"context"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/outbox"
	"github.com/shortlink-org/go-sdk/uow"
)

// TestPublishRolledBackLeavesNothingToDeliver is the property the whole
// pattern exists for: the message and the state change it describes commit
// together or not at all.
func TestPublishRolledBackLeavesNothingToDeliver(t *testing.T) {
	pool := setupPostgres(t)

	inTx(t, pool, func(ctx context.Context) {
		publisher, err := outbox.NewPublisher(uow.FromContext)
		require.NoError(t, err)

		require.NoError(t, publisher.Publish(ctx, testTopic, message.NewMessage("rolled-back", []byte(`{"id":1}`))))

		// The row exists for the transaction that wrote it, which is what
		// makes the count after the rollback meaningful.
		var count int

		require.NoError(t, uow.FromContext(ctx).QueryRow(ctx, "SELECT count(*) FROM outbox").Scan(&count))
		require.Equal(t, 1, count)
	}, false)

	require.Equal(t, 0, countRows(t, pool, "TRUE"), "a rolled back transaction must leave no message behind")
}

// TestPublishOutsideTransactionFails covers the mistake this package refuses
// to paper over: publishing after the commit rather than inside it. Writing on
// a pooled connection would succeed and outlive a rollback, which is the exact
// failure the outbox prevents.
func TestPublishOutsideTransactionFails(t *testing.T) {
	pool := setupPostgres(t)

	publisher, err := outbox.NewPublisher(uow.FromContext)
	require.NoError(t, err)

	err = publisher.Publish(context.Background(), testTopic, message.NewMessage("orphan", []byte(`{"id":1}`)))
	require.ErrorIs(t, err, outbox.ErrNoTransaction)

	require.Equal(t, 0, countRows(t, pool, "TRUE"), "the failed publish must not have written anything")
}

// TestPublishRequiresATopic keeps an empty topic from becoming a row nothing
// will ever claim.
func TestPublishRequiresATopic(t *testing.T) {
	pool := setupPostgres(t)

	inTx(t, pool, func(ctx context.Context) {
		publisher, err := outbox.NewPublisher(uow.FromContext)
		require.NoError(t, err)

		require.ErrorIs(t,
			publisher.Publish(ctx, "", message.NewMessage("no-topic", []byte("x"))),
			outbox.ErrNoTopic,
		)
	}, true)

	require.Equal(t, 0, countRows(t, pool, "TRUE"))
}

// TestPublishCarriesMetadata checks the round trip of the fields the router's
// middleware relies on — correlation id among them.
func TestPublishCarriesMetadata(t *testing.T) {
	pool := setupPostgres(t)

	msg := message.NewMessage("with-metadata", []byte(`{"id":1}`))
	msg.Metadata.Set("correlation_id", "abc-123")

	publish(t, pool, testTopic, msg)

	var (
		uuid     string
		metadata []byte
	)

	err := pool.QueryRow(context.Background(),
		"SELECT uuid, metadata FROM outbox WHERE topic = $1", testTopic,
	).Scan(&uuid, &metadata)
	require.NoError(t, err)

	require.Equal(t, "with-metadata", uuid)
	require.JSONEq(t, `{"correlation_id":"abc-123"}`, string(metadata))
}
