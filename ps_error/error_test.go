package ps_error_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/crylabsorg/prostack-go/ps_error"
)

func TestPSError(t *testing.T) {
	t.Parallel()
	cause := errors.New("cause")
	err := ps_error.PSError{ErrorCode: ps_error.ErrorCodeValidationFailed, OriginalError: cause}
	var got ps_error.PSError

	require.ErrorAs(t, err, &got)
	require.Equal(t, ps_error.ErrorCodeValidationFailed, got.ErrorCode)
	require.ErrorIs(t, err, cause)
	require.NotErrorIs(t, err, ps_error.PSError{ErrorCode: ps_error.ErrorCodeGeneral})
}

func TestPSErrorGRPCDetails(t *testing.T) {
	t.Parallel()
	err := ps_error.PSError{
		ErrorCode: ps_error.ErrorCodeValidationFailed,
		Message:   "validation failed",
		GStatus:   codes.InvalidArgument,
	}

	grpcErr := err.ToGRPCError()
	require.True(t, ps_error.IsErrorCode(grpcErr, ps_error.ErrorCodeValidationFailed))
	require.False(t, ps_error.IsErrorCode(grpcErr, ps_error.ErrorCodeGeneral))
}

func TestPSErrorGRPCFallback(t *testing.T) {
	t.Parallel()
	for _, err := range []ps_error.PSError{
		{GStatus: codes.OK},
		{GStatus: codes.InvalidArgument, Args: []any{struct{}{}}},
	} {
		grpcErr := err.ToGRPCError()
		require.Equal(t, codes.Internal, status.Code(grpcErr))
		require.True(t, ps_error.IsErrorCode(grpcErr, ps_error.ErrorCodeGeneral))
	}
}
