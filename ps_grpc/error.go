//nolint:nolintlint,revive // ignore
package ps_grpc

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/crylabsorg/prostack-go/ps_error"
)

func ErrorInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	resp, err := handler(ctx, req)
	if err != nil {
		if validationError, ok := errors.AsType[validator.ValidationErrors](err); ok {
			return nil, ps_error.ValidationErrorToPSError(validationError).ToGRPCError()
		}

		var suErr ps_error.PSError
		var suErr1 *ps_error.PSError
		switch {
		case errors.As(err, &suErr):
			return nil, suErr.ToGRPCError()
		case errors.As(err, &suErr1):
			return nil, suErr1.ToGRPCError()
		}
		return nil, ps_error.PSError{
			ErrorCode:     ps_error.ErrorCodeGeneral,
			Message:       "internal server error",
			Args:          nil,
			HStatus:       http.StatusInternalServerError,
			GStatus:       codes.Internal,
			OriginalError: err,
		}.ToGRPCError()
	}
	return resp, err
}
