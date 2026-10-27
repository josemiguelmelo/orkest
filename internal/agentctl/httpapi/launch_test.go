package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
)

func TestLaunchRequiresConfiguration(t *testing.T) {
	service := agentrun.NewRunController(context.Background(), nil, "", nil)
	api, err := NewServer(context.Background(), service)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	api.Handler(nil).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/agent/launch", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("got %d", response.Code)
	}
}
