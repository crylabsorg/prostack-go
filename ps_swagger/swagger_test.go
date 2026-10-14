package ps_swagger_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/crylabsorg/prostack-go/ps_swagger"
)

func TestSetupSwagger(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	spec := []byte(`{"openapi":"3.1.0"}`)
	ps_swagger.SetupSwagger(t.Context(), mux, spec)

	t.Run("redirect to directory", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/swagger", nil))

		require.Equal(t, http.StatusTemporaryRedirect, response.Code)
		require.Equal(t, "/swagger/", response.Header().Get("Location"))
	})

	for _, path := range []string{
		"/swagger/",
		"/swagger/swagger-ui.css",
		"/swagger/swagger-ui-bundle.js",
		"/swagger/swagger-ui-standalone-preset.js",
		"/swagger/favicon-32x32.png",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))

			require.Equal(t, http.StatusOK, response.Code)
			require.NotEmpty(t, response.Body.Bytes())
		})
	}

	t.Run("specification", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/swagger.json", nil))

		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "application/json", response.Header().Get("Content-Type"))
		require.Equal(t, spec, response.Body.Bytes())
	})
}
