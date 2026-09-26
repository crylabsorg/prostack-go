package ps_grpc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crylabsorg/prostack-go/controller"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErrorInterceptorHidesUnexpectedError(t *testing.T) {
	_, err := ErrorInterceptor(context.Background(), nil, nil, func(context.Context, any) (any, error) {
		return nil, errors.New("database connection failed")
	})
	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "internal server error", status.Convert(err).Message())

	response := httptest.NewRecorder()
	controller.NewController().WriteError(context.Background(), response, err)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.NotContains(t, response.Body.String(), "database connection failed")
}
