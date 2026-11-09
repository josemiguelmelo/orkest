package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"

	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
)

const validConfig = `{"organizationId":"org","workspaceId":"workspace","taskId":"task","runId":"run","runtime":"opencode","model":{"provider":"provider","id":"model"},"agent":{"name":"Work","description":"Work","instructions":"Work on this task"}}`

func setupRESTServer(t *testing.T) *Server {
	t.Helper()
	server, err := NewServer(context.Background(), agentrun.NewRunController(context.Background(), nil, "", nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func request(s *Server, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler(nil).ServeHTTP(w, r)
	return w
}

func TestHTTPContract(t *testing.T) {
	s := setupRESTServer(t)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/health", "", 200},
		{"POST", "/health", "", 405},
		{"GET", "/info", "", 200},
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
	var info wire.Info
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

func TestConfigureLargeBodyAndExtraFields(t *testing.T) {
	s := setupRESTServer(t)
	body := strings.Replace(validConfig, "Work on this task", strings.Repeat("x", 128<<10), 1)
	body = strings.TrimSuffix(body, "}") + `,"extra":"ignored"}`
	response := request(s, "POST", "/agent/configure", body)
	if response.Code != http.StatusOK {
		t.Fatalf("configuration rejected: %d %s", response.Code, response.Body.String())
	}
}
