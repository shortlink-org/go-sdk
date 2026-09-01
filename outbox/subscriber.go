package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// claimSQL takes the oldest undelivered rows of one topic and locks them for
// this transaction. SKIP LOCKED is what makes a second relay useful rather
// than merely blocked: it walks past the rows the first one is holding and
// takes the next ones.
//
//nolint:gosec // tableName is a package constant, not user input
var claimSQL = fmt.Sprintf(
	`SELECT id, uuid, payload, metadata
	   FROM %s
	  WHERE topic = $1 AND delivered_at IS NULL
	  ORDER BY id
	  LIMIT $2
	    FOR UPDATE SKIP LOCKED`,
	tableName,
)

//nolint:gosec // tableName is a package constant, not user input
var markDeliveredSQL = fmt.Sprintf(
	`UPDATE %s SET delivered_at = now() WHERE id = ANY($1)`,
	tableName,
)

// subscriber reads the outbox table and feeds a Watermill router.
//
// It is one subscriber for every topic the relay handles: Subscribe starts a
// read loop of its own per topic, and the loops share nothing but the pool.
type subscriber struct {
	pool *pgxpool.Pool
	log  *slog.Logger
	opts Options

	closing   chan struct{}
	closeOnce sync.Once
	loops     sync.WaitGroup
}

func newSubscriber(pool *pgxpool.Pool, log *slog.Logger, opts Options) *subscriber { //nolint:gocritic // Options is public API
	return &subscriber{
		pool:      pool,
		log:       log,
		opts:      opts,
		closing:   make(chan struct{}),
		closeOnce: sync.Once{},
		loops:     sync.WaitGroup{},
	}
}

// Subscribe implements message.Subscriber.
func (s *subscriber) Subscribe(ctx context.Context, topic string) (<-chan *message.Message, error) {
	out := make(chan *message.Message)

	s.loops.Go(func() {
		defer close(out)

		s.poll(ctx, topic, out)
	})

	return out, nil
}

// Close implements message.Subscriber. The router calls it once per handler,
// so it has to tolerate repetition.
func (s *subscriber) Close() error {
	s.closeOnce.Do(func() {
		close(s.closing)
	})

	s.loops.Wait()

	return nil
}

func (s *subscriber) poll(ctx context.Context, topic string, out chan<- *message.Message) {
	for {
		if ctx.Err() != nil {
			return
		}

		select {
		case <-s.closing:
			return
		default:
		}

		claimed, err := s.claim(ctx, topic, out)
		if err != nil {
			// A canceled context is the shutdown path, not a failure.
			if ctx.Err() != nil {
				return
			}

			s.log.Error("outbox: failed to read the outbox",
				slog.String("topic", topic),
				slog.String("error", err.Error()),
			)
		}

		// A full batch means there is probably more behind it; only an
		// incomplete read is worth waiting after.
		if err == nil && claimed == s.opts.BatchSize {
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-s.closing:
			return
		case <-time.After(s.opts.PollInterval):
		}
	}
}

// claimedRow pairs a message with the row it came from, because the row is
// what gets marked delivered.
type claimedRow struct {
	id  int64
	msg *message.Message
}

// claim reads one batch, hands it to the router, and marks delivered whatever
// came back acknowledged. Everything else — nacked, or interrupted — is left
// as it was: the rollback releases the locks and the next read sees the rows
// again. That is the at-least-once boundary, and it is why handlers have to be
// idempotent.
func (s *subscriber) claim(ctx context.Context, topic string, out chan<- *message.Message) (int, error) {
	transaction, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("outbox: begin: %w", err)
	}

	defer func() {
		// The rollback must run even when ctx is already canceled, or the
		// rows stay locked until the connection is reaped.
		_ = transaction.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck // cleanup path
	}()

	batch, err := s.read(ctx, transaction, topic)
	if err != nil {
		return 0, err
	}

	if len(batch) == 0 {
		return 0, nil
	}

	// Hand the whole batch over before waiting on any of it. The router runs
	// each message in its own goroutine, so waiting message by message would
	// serialize a batch that has no ordering to preserve.
	for _, row := range batch {
		row.msg.SetContext(ctx)

		select {
		case out <- row.msg:
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-s.closing:
			return 0, ErrRelayClosed
		}
	}

	delivered := make([]int64, 0, len(batch))

	for _, row := range batch {
		select {
		case <-row.msg.Acked():
			delivered = append(delivered, row.id)
		case <-row.msg.Nacked():
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-s.closing:
			return 0, ErrRelayClosed
		}
	}

	if len(delivered) > 0 {
		_, err = transaction.Exec(ctx, markDeliveredSQL, delivered)
		if err != nil {
			return 0, fmt.Errorf("outbox: mark delivered: %w", err)
		}
	}

	err = transaction.Commit(ctx)
	if err != nil {
		return 0, fmt.Errorf("outbox: commit: %w", err)
	}

	return len(batch), nil
}

func (s *subscriber) read(ctx context.Context, transaction pgx.Tx, topic string) ([]claimedRow, error) {
	rows, err := transaction.Query(ctx, claimSQL, topic, s.opts.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("outbox: claim: %w", err)
	}

	defer rows.Close()

	batch := make([]claimedRow, 0, s.opts.BatchSize)

	for rows.Next() {
		var (
			messageID int64
			uuid      string
			payload   []byte
			metadata  []byte
		)

		err = rows.Scan(&messageID, &uuid, &payload, &metadata)
		if err != nil {
			return nil, fmt.Errorf("outbox: scan: %w", err)
		}

		msg := message.NewMessage(uuid, payload)

		err = decodeMetadata(metadata, msg)
		if err != nil {
			return nil, err
		}

		batch = append(batch, claimedRow{id: messageID, msg: msg})
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("outbox: read: %w", err)
	}

	return batch, nil
}

func decodeMetadata(raw []byte, msg *message.Message) error {
	if len(raw) == 0 {
		return nil
	}

	fields := map[string]string{}

	err := json.Unmarshal(raw, &fields)
	if err != nil {
		return fmt.Errorf("outbox: unmarshal metadata of %s: %w", msg.UUID, err)
	}

	for key, value := range fields {
		msg.Metadata.Set(key, value)
	}

	return nil
}
