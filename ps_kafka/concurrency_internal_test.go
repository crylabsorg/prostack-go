package ps_kafka //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestProcessRecordsConcurrentHandlersAndOrderedCommits(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		records := []*kgo.Record{
			{Topic: "events", Partition: 0, Offset: 0},
			{Topic: "events", Partition: 0, Offset: 1},
			{Topic: "events", Partition: 1, Offset: 0},
		}
		started := make(chan struct{}, len(records))
		commits := make(chan *kgo.Record, len(records))
		releaseFirst := make(chan struct{})
		finished := make(chan error, 1)
		go func() {
			finished <- processRecords(context.Background(), records, "test",
				func(_ context.Context, record *kgo.Record) error {
					started <- struct{}{}
					if record == records[0] {
						<-releaseFirst
					}
					return nil
				},
				func(_ context.Context, record *kgo.Record) error {
					commits <- record
					return nil
				})
		}()
		for range records {
			<-started
		}
		synctest.Wait()
		// The other partition can commit while offset 1 waits for offset 0.
		require.Len(t, commits, 1)
		require.Same(t, records[2], <-commits)
		require.Empty(t, finished)
		close(releaseFirst)
		require.NoError(t, <-finished)
		require.Len(t, commits, 2)
		require.Same(t, records[0], <-commits)
		require.Same(t, records[1], <-commits)
	})
}

func TestProcessRecordsDoesNotCommitPastFailure(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"handler", "commit"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			records := []*kgo.Record{
				{Topic: "events", Partition: 0, Offset: 0},
				{Topic: "events", Partition: 0, Offset: 1},
				{Topic: "events", Partition: 0, Offset: 2},
				{Topic: "events", Partition: 1, Offset: 0},
			}
			failureErr := errors.New("delivery failed")
			handled := make(chan *kgo.Record, len(records))
			commits := make(chan *kgo.Record, len(records))
			err := processRecords(context.Background(), records, "test",
				func(_ context.Context, record *kgo.Record) error {
					handled <- record
					if failure == "handler" && record == records[1] {
						return failureErr
					}
					return nil
				},
				func(_ context.Context, record *kgo.Record) error {
					commits <- record
					if failure == "commit" && record == records[1] {
						return failureErr
					}
					return nil
				})
			require.ErrorIs(t, err, failureErr)
			require.Len(t, handled, len(records))
			close(commits)
			var attempted []*kgo.Record
			for record := range commits {
				attempted = append(attempted, record)
			}
			expected := []*kgo.Record{records[0], records[3]}
			if failure == "commit" {
				expected = append(expected, records[1])
			}
			require.ElementsMatch(t, expected, attempted)
			require.ErrorContains(t, err, "offset commit blocked")
		})
	}
}

func TestProcessRecordsCancellationWaitsForHandlers(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		records := []*kgo.Record{
			{Topic: "events", Partition: 0, Offset: 0},
			{Topic: "events", Partition: 0, Offset: 1},
		}
		started := make(chan struct{}, len(records))
		commits := make(chan *kgo.Record, len(records))
		releaseFirst := make(chan struct{})
		finished := make(chan error, 1)
		go func() {
			finished <- processRecords(ctx, records, "test",
				func(ctx context.Context, record *kgo.Record) error {
					started <- struct{}{}
					if record == records[0] {
						<-releaseFirst
						return ctx.Err()
					}
					return nil
				},
				func(_ context.Context, record *kgo.Record) error {
					commits <- record
					return nil
				})
		}()
		for range records {
			<-started
		}
		synctest.Wait()
		cancel()
		synctest.Wait()
		require.Empty(t, commits)
		require.Empty(t, finished, "closing the client must wait for every active handler")
		close(releaseFirst)
		require.ErrorIs(t, <-finished, context.Canceled)
		require.Empty(t, commits)
	})
}
