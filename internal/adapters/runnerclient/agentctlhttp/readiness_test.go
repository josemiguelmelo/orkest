package agentctlhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
)

type unusedConnectionRepository struct {
	orchestration.ConnectionRepository
}

func TestUpgradedPeerWithoutReadyFrameTimesOutAndDisconnects(t *testing.T) {
	exited := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer close(exited)
		defer func() { _ = conn.CloseNow() }()
		_, _, _ = conn.Read(r.Context())
	}))
	defer server.Close()
	client := NewHTTPAgentctlClient(nil)
	service, err := orchestration.NewConnectionService(context.Background(), client, unusedConnectionRepository{}, nil, orchestration.WithConnectionReadinessTimeout(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = service.Start(ctx, orchestration.Launch{RunID: "run", Endpoint: server.URL})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("readiness = %v", err)
	}
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("unready peer did not disconnect")
	}
}
