package opencode

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func conversationService(t *testing.T) *agentctl.RunController {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	provider := &Provider{Binary: binary, WorkDir: t.TempDir(), StateDir: state, Env: []string{"ORKEST_ACP_FIXTURE=1"}}
	service := agentctl.NewRunControllerWithProvider(context.Background(), provider, state)
	t.Cleanup(func() { _ = service.Close() })
	config := agentctl.RunConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: agentctl.Model{Provider: "provider", ID: "model"}, SystemPrompt: "Work"}
	if err := service.Configure(config); err != nil {
		t.Fatal(err)
	}
	if err := service.Launch(); err != nil {
		t.Fatal(err)
	}
	return service
}

func nextEvent(t *testing.T, events <-chan agentctl.Event, kind agentctl.EventType) agentctl.Event {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("event subscription closed")
			}
			if event.Type == kind {
				return event
			}
		case <-timer.C:
			t.Fatalf("missing event %s", kind)
		}
	}
}

func TestConversationPermissionReplayAndIdempotency(t *testing.T) {
	service := conversationService(t)
	_, events, stop, err := service.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err := service.Prompt("cmd-1", "permission"); err != nil {
		t.Fatal(err)
	}
	accepted := nextEvent(t, events, "prompt_accepted")
	// ACP requests and notifications are dispatched independently. Both must
	// arrive, but their relative order is not guaranteed.
	var update, permission agentctl.Event
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for update.Type == "" || permission.Type == "" {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("subscription closed")
			}
			switch event.Type {
			case "update":
				update = event
			case "permission_requested":
				permission = event
			}
		case <-deadline.C:
			t.Fatal("missing update or permission request")
		}
	}
	if update.Update.Text != "Hello from fixture" {
		t.Fatalf("missing streamed text: %+v", update)
	}
	if permission.Permission.ToolCallID != "tool-1" || len(permission.Permission.Options) != 2 {
		t.Fatal("permission details missing")
	}
	if err := service.Prompt("cmd-1", "permission"); err != nil {
		t.Fatal("identical command was not idempotent", err)
	}
	if err := service.Prompt("cmd-1", "different"); !errors.Is(err, agentctl.ErrCommand) {
		t.Fatal("changed command accepted")
	}
	if err := service.Prompt("cmd-2", "hello"); !errors.Is(err, agentctl.ErrConversation) {
		t.Fatal("concurrent prompt accepted")
	}
	response := agentctl.PermissionResponse{TaskID: "task", RunID: "other-run", SessionID: permission.SessionID, RequestID: permission.RequestID, OptionID: "allow"}
	if err := service.RespondPermission(response); !errors.Is(err, agentctl.ErrPermission) {
		t.Fatal("wrong run accepted")
	}
	response.RunID = "run"
	response.OptionID = "unoffered"
	if err := service.RespondPermission(response); !errors.Is(err, agentctl.ErrPermission) {
		t.Fatal("unoffered option accepted")
	}
	// Disconnect/reconnect while approval waits. No response is synthesized.
	stop()
	replay, events, stop, err := service.Subscribe(accepted.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if len(replay) != 2 || (replay[0].Type != "permission_requested" && replay[1].Type != "permission_requested") {
		t.Fatal("pending permission not replayed")
	}
	if err := service.RespondPermission(agentctl.PermissionResponse{TaskID: "task", RunID: "run", SessionID: permission.SessionID, RequestID: permission.RequestID, OptionID: "reject"}); err != nil {
		t.Fatal(err)
	}
	completed := nextEvent(t, events, "prompt_completed")
	if completed.StopReason != "end_turn" {
		t.Fatalf("unexpected stop reason %s", completed.StopReason)
	}
	if err := service.RespondPermission(response); !errors.Is(err, agentctl.ErrPermission) {
		t.Fatal("stale answer accepted")
	}
	if err := service.Prompt("cmd-2", "hello"); err != nil {
		t.Fatal(err)
	}
	nextEvent(t, events, "prompt_completed")
}

func TestConversationCancel(t *testing.T) {
	service := conversationService(t)
	_, events, stop, err := service.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err := service.Prompt("cancel-me", "wait"); err != nil {
		t.Fatal(err)
	}
	nextEvent(t, events, "update")
	if err := service.CancelPrompt("wrong"); !errors.Is(err, agentctl.ErrCommand) {
		t.Fatal("wrong command cancelled")
	}
	if err := service.CancelPrompt("cancel-me"); err != nil {
		t.Fatal(err)
	}
	completed := nextEvent(t, events, "prompt_completed")
	if completed.StopReason != "cancelled" {
		t.Fatalf("unexpected stop reason %s", completed.StopReason)
	}
}
