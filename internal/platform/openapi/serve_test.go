package openapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestEnableServesSwaggerAssetsAndContract(t *testing.T) {
	const contract = "openapi: 3.1.0\n"
	router := chi.NewRouter()
	Enable(router, "Test API", []byte(contract))

	for _, path := range []string{"/docs/", "/docs/swagger-ui.css", "/docs/swagger-ui-bundle.js"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != 200 {
			t.Errorf("GET %s returned %d", path, recorder.Code)
		}
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/openapi.yaml", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), contract) {
		t.Fatalf("GET /openapi.yaml returned %d with body %q", recorder.Code, recorder.Body.String())
	}
}
