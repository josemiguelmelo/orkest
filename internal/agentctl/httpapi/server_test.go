package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"
)

func TestTransportModulesShareState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server, err := NewServer(ctx, agentctl.NewRunController())
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/agent/stream", nil))
	if response.Code != http.StatusConflict {
		t.Fatal("unconfigured stream was accepted")
	}
	body := `{"organizationId":"org","workspaceId":"workspace","taskId":"task","runId":"run","runtime":"opencode","model":{"provider":"provider","id":"model"},"agent":{"name":"Work","description":"Work","instructions":"Work"}}`
	request := httptest.NewRequest(http.MethodPost, "/agent/configure", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("configure: %s", response.Body)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/info", nil))
	var info protocol.Info
	if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/agent/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	var ready struct {
		Type string        `json:"type"`
		Info protocol.Info `json:"info"`
	}
	if err := wsjson.Read(ctx, conn, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Type != "ready" || !ready.Info.Configured || ready.Info != info {
		t.Fatalf("transport snapshots disagree: REST %+v, WebSocket %+v", info, ready)
	}
}

func TestServiceRequired(t *testing.T) {
	if _, err := NewServer(context.Background(), nil); err == nil {
		t.Fatal("accepted missing service")
	}
}
