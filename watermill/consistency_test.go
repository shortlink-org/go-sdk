//go:build unit

package watermill

import (
	"context"
	"errors"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingPublisher struct {
	messages []*message.Message
}

func (p *recordingPublisher) Publish(_ string, messages ...*message.Message) error {
	p.messages = append(p.messages, messages...)

	return nil
}

func (*recordingPublisher) Close() error { return nil }

type failingWatermarker struct{}

func (failingWatermarker) Capture(context.Context) (string, error) {
	return "", errors.New("primary unavailable")
}

type emptyWatermarker struct{}

func (emptyWatermarker) Capture(context.Context) (string, error) {
	return "", nil
}

func TestConsistencyPublisherCarriesAnUnresolvedWrite(t *testing.T) {
	t.Parallel()

	inner := &recordingPublisher{}
	publisher := NewConsistencyPublisher(inner, failingWatermarker{}, nil, nil)
	msg := message.NewMessage("id", nil)

	require.NoError(t, publisher.Publish("events", msg))
	require.Len(t, inner.messages, 1)
	assert.Equal(t, unresolvedWatermark, inner.messages[0].Metadata.Get(MetaWALWatermark))
}

func TestConsistencyPublisherLeavesCleanMessagesUnstamped(t *testing.T) {
	t.Parallel()

	inner := &recordingPublisher{}
	publisher := NewConsistencyPublisher(inner, emptyWatermarker{}, nil, nil)
	msg := message.NewMessage("id", nil)

	require.NoError(t, publisher.Publish("events", msg))
	require.Len(t, inner.messages, 1)
	assert.Empty(t, inner.messages[0].Metadata.Get(MetaWALWatermark))
}
