package ps_event

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFanoutAcknowledgesOnlyWhenAllPublishersSucceed(t *testing.T) {
	firstErr, secondErr := errors.New("nats failed"), errors.New("kafka failed")
	for _, failures := range [][]error{{nil, nil}, {firstErr, nil}, {nil, secondErr}, {firstErr, secondErr}} {
		calls := 0
		publishers := Fanout{}
		for _, failure := range failures {
			publishers = append(publishers, testPublisher(func(ctx context.Context, message OutboxMessage) error {
				calls++
				require.Equal(t, "event-id", message.ID())
				return failure
			}))
		}
		err := publishers.Publish(context.Background(), testOutboxMessage{"event-id"})
		require.Equal(t, 2, calls)
		if failures[0] == nil && failures[1] == nil {
			require.NoError(t, err)
		}
		for _, failure := range failures {
			if failure != nil {
				require.ErrorIs(t, err, failure)
			}
		}
	}
	require.Error(t, Fanout{}.Publish(context.Background(), testOutboxMessage{"event-id"}))
}
