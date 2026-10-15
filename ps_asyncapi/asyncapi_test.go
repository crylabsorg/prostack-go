package ps_asyncapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/crylabsorg/prostack-go/ps_asyncapi"
)

func TestDocumentationRoutes(t *testing.T) {
	t.Parallel()

	specification := []byte(`{"asyncapi":"3.0.0","info":{"title":"Events","version":"1.0.0"}}`)
	mux := http.NewServeMux()
	ps_asyncapi.SetupAsyncAPI(t.Context(), mux, specification)
	for _, test := range []struct {
		path        string
		contentType string
	}{
		{"/asyncapi", "text/html"},
		{"/asyncapi/", "text/html"},
		{"/asyncapi.json", "application/json"},
		{"/asyncapi/asyncapi-ui.js", "javascript"},
		{"/asyncapi/asyncapi-ui.css", "text/css"},
		{"/asyncapi/LICENSE", "text/plain"},
		{"/asyncapi/index.js.LICENSE.txt", "text/plain"},
	} {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			require.Equal(t, http.StatusOK, response.Code)
			require.Contains(t, response.Header().Get("Content-Type"), test.contentType)
			require.NotEmpty(t, response.Body.Bytes())

			if test.contentType == "application/json" {
				require.Equal(t, specification, response.Body.Bytes())
			}

			if test.contentType == "text/html" {
				require.Contains(t, response.Body.String(), "/asyncapi.json")
				require.Contains(t, response.Body.String(), "/asyncapi/asyncapi-ui.js")
				require.NotContains(t, response.Body.String(), "https://")
			}

			head := httptest.NewRecorder()
			mux.ServeHTTP(head, httptest.NewRequest(http.MethodHead, test.path, nil))
			require.Equal(t, http.StatusOK, head.Code)
			require.Empty(t, head.Body.Bytes())
		})
	}

	for _, path := range []string{"/asyncapi", "/asyncapi/", "/asyncapi/asyncapi-ui.js", "/asyncapi.json"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		require.Equal(t, http.StatusMethodNotAllowed, response.Code)
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/asyncapi/missing.js", nil))
	require.Equal(t, http.StatusNotFound, response.Code)
}
