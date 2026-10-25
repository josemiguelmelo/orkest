package rest

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestRESTCommandErrors(t *testing.T) {
	server := setup(t)
	for _, tc := range []struct {
		path, body, code string
		status           int
	}{
		{"/agent/prompt", `{}`, "invalid_command", http.StatusBadRequest},
		{"/agent/prompt", `{"commandId":"first","text":"hello"}`, "conversation_unavailable", http.StatusConflict},
		{"/agent/cancel", `{}`, "invalid_command", http.StatusBadRequest},
		{"/agent/permissions/respond", `{}`, "invalid_permission", http.StatusConflict},
	} {
		response := request(server, http.MethodPost, tc.path, tc.body)
		var body struct {
			Code string `json:"errorCode"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != tc.status || body.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.path, response.Code, response.Body)
		}
	}
	for _, path := range []string{"/agent/prompt", "/agent/cancel", "/agent/permissions/respond"} {
		if response := request(server, http.MethodPost, path, `{`); response.Code != http.StatusBadRequest {
			t.Fatalf("invalid JSON at %s: %d", path, response.Code)
		}
		if response := request(server, http.MethodGet, path, ""); response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s: %d", path, response.Code)
		}
	}
}
