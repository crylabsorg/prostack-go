package ps_kafka //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestProcessRecordDoesNotCommitOnFailure(t *testing.T) {
	t.Parallel()
	handleErr, commitErr := errors.New("handle failed"), errors.New("commit failed")
	for _, failure := range []error{handleErr, nil} {
		committed := false
		err := processRecord(
			context.Background(),
			&kgo.Record{Topic: "user.created"},
			func(context.Context, *kgo.Record) error {
				return failure
			},
			func(context.Context, *kgo.Record) error {
				committed = true
				return commitErr
			},
		)
		require.Equal(t, failure == nil, committed)
		if failure != nil {
			require.ErrorIs(t, err, handleErr)
		} else {
			require.ErrorIs(t, err, commitErr)
		}
	}
}
