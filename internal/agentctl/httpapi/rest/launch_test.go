package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func TestLaunchRequiresConfiguration(t *testing.T) {
	service := agentctl.NewRunController()
	api, err := NewServer(service, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/agent/launch", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("got %d", response.Code)
	}
}
