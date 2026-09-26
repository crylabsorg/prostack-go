package ps_error

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPSError(t *testing.T) {
	cause := errors.New("cause")
	err := PSError{ErrorCode: ErrorCodeValidationFailed, OriginalError: cause}
	var got PSError

	require.ErrorAs(t, err, &got)
	require.Equal(t, ErrorCodeValidationFailed, got.ErrorCode)
	require.ErrorIs(t, err, cause)
	require.NotErrorIs(t, err, PSError{ErrorCode: ErrorCodeGeneral})
}

func TestPSErrorGRPCDetails(t *testing.T) {
	err := PSError{
		ErrorCode: ErrorCodeValidationFailed,
		Message:   "validation failed",
		GStatus:   codes.InvalidArgument,
	}

	grpcErr := err.ToGRPCError()
	require.True(t, IsErrorCode(grpcErr, ErrorCodeValidationFailed))
	require.False(t, IsErrorCode(grpcErr, ErrorCodeGeneral))
}

func TestPSErrorGRPCFallback(t *testing.T) {
	for _, err := range []PSError{
		{GStatus: codes.OK},
		{GStatus: codes.InvalidArgument, Args: []any{struct{}{}}},
	} {
		grpcErr := err.ToGRPCError()
		require.Equal(t, codes.Internal, status.Code(grpcErr))
		require.True(t, IsErrorCode(grpcErr, ErrorCodeGeneral))
	}
}
