package ps_grpc_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/crylabsorg/prostack-go/ps_grpc"

	"github.com/crylabsorg/prostack-go/ps_controller"
)

func TestErrorInterceptorHidesUnexpectedError(t *testing.T) {
	t.Parallel()
	_, err := ps_grpc.ErrorInterceptor(context.Background(), nil, nil, func(context.Context, any) (any, error) {
		return nil, errors.New("database connection failed")
	})
	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "internal server error", status.Convert(err).Message())

	response := httptest.NewRecorder()
	ps_controller.NewController().WriteError(context.Background(), response, err)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.NotContains(t, response.Body.String(), "database connection failed")
}
