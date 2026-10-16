package ps_controller_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/crylabsorg/prostack-go/ps_controller"
)

func TestWriteJSONEncodingError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()

	ps_controller.NewController().WriteJSON(context.Background(), w, http.StatusOK, make(chan int))

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	require.JSONEq(t, `{"message":"internal server error","errorCode":1}`, w.Body.String())
}
