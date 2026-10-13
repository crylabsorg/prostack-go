package ps_event

import (
	"context"
	"errors"
)

// Publisher returns nil only after the broker acknowledges the message.
// Deliveries are at least once; consumers must tolerate repeated message IDs.
type Publisher interface {
	Publish(context.Context, OutboxMessage) error
}

// Fanout attempts every publisher. The outbox completes a message only when all
// publishers succeed. A partial failure retries delivery to every publisher.
// ponytail: use per-broker outbox state if independent retries become necessary.
type Fanout []Publisher

func (publishers Fanout) Publish(ctx context.Context, message OutboxMessage) error {
	if len(publishers) == 0 {
		return errors.New("no event publishers configured")
	}
	var publishErr error
	for _, publisher := range publishers {
		publishErr = errors.Join(publishErr, publisher.Publish(ctx, message))
	}
	return publishErr
}
