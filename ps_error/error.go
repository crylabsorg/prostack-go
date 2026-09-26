package ps_error

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	pbError "github.com/crylabsorg/prostack-go/pkg/proto/prostack/error/v1"
)

type PSError struct {
	ErrorCode     ErrorCode `json:"errorCode"`
	Message       string    `json:"message"`
	Args          []any     `json:"args"`
	HStatus       int32
	GStatus       codes.Code
	OriginalError error
}

func (e PSError) ToGRPCError() error {
	st := status.New(e.GStatus, e.Message)
	listArgs, err := structpb.NewList(e.Args)
	if err != nil {
		err1 := PSError{
			ErrorCode:     ErrorCodeGeneral,
			Message:       "Internal server error",
			Args:          nil,
			HStatus:       http.StatusInternalServerError,
			GStatus:       codes.Internal,
			OriginalError: err,
		}
		return err1.ToGRPCError()
	}
	pbErr := pbError.Error{}
	pbErr.SetErrorCode(int32(e.ErrorCode))
	pbErr.SetHttpStatus(e.HStatus)
	pbErr.SetArgs(listArgs)
	st, err = st.WithDetails(&pbErr)
	if err != nil {
		err1 := PSError{
			ErrorCode:     ErrorCodeGeneral,
			Message:       "Internal server error",
			Args:          nil,
			HStatus:       http.StatusInternalServerError,
			GStatus:       codes.Internal,
			OriginalError: err,
		}
		return err1.ToGRPCError()
	}
	return st.Err()
}

func (e PSError) Error() string {
	return e.Message
}

func (e PSError) Unwrap() error {
	return e.OriginalError
}
