package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

const validConfig = `{"organizationId":"org","workspaceId":"workspace","taskId":"task","runId":"run","runtime":"opencode","model":{"provider":"provider","id":"model"},"systemPrompt":"Work on this task"}`

func setup(t *testing.T) (*Server, *httptest.Server, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s, err := NewServer(ctx, agentctl.NewService())
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(s.Handler())
	t.Cleanup(func() { cancel(); httpServer.Close() })
	return s, httpServer, cancel
}

func request(s *Server, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestHTTPContract(t *testing.T) {
	s, _, _ := setup(t)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/health", "", 200},
		{"POST", "/health", "", 405},
		{"GET", "/info", "", 200},
		{"GET", "/agent/stream", "", 409},
		{"POST", "/agent/configure", `{}`, 400},
		{"POST", "/agent/configure", `null`, 400},
		{"POST", "/agent/configure", validConfig + ` {}`, 400},
		{"POST", "/agent/configure", strings.Replace(validConfig, `"runtime"`, `"unknown"`, 1), 400},
		{"POST", "/agent/configure", strings.Replace(validConfig, "opencode", "", 1), 400},
		{"POST", "/agent/configure", validConfig, 200},
		{"POST", "/agent/configure", validConfig, 200},
		{"POST", "/agent/configure", strings.Replace(validConfig, `"run"`, `"other-run"`, 1), 409},
	} {
		if got := request(s, tc.method, tc.path, tc.body); got.Code != tc.status {
			t.Errorf("%s %s: got %d want %d: %s", tc.method, tc.path, got.Code, tc.status, got.Body.String())
		}
	}
	w := request(s, "GET", "/info", "")
	var info Info
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if !info.Configured || info.ExecutionAvailable || info.Service != "agentctl" {
		t.Fatalf("unexpected info: %+v", info)
	}
	if strings.Contains(w.Body.String(), "Work on") {
		t.Fatal("info leaked configuration")
	}
}

func TestServiceRequired(t *testing.T) {
	if _, err := NewServer(context.Background(), nil); err == nil {
		t.Fatal("accepted missing service")
	}
}

func TestConfigureLargeBodyAndExtraFields(t *testing.T) {
	s, _, _ := setup(t)
	body := strings.Replace(validConfig, "Work on this task", strings.Repeat("x", 128<<10), 1)
	body = strings.TrimSuffix(body, "}") + `,"extra":"ignored"}`
	response := request(s, "POST", "/agent/configure", body)
	if response.Code != http.StatusOK {
		t.Fatalf("configuration rejected: %d %s", response.Code, response.Body.String())
	}
}
