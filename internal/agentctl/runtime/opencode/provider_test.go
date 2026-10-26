package opencode

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func TestMain(m *testing.M) {
	if os.Getenv("ORKEST_ACP_FIXTURE") == "1" {
		if os.Args[1] == "--version" {
			if os.Getenv("ORKEST_ACP_WRONG_VERSION") == "1" {
				fmt.Println("SECRET")
				os.Exit(0)
			}
			fmt.Println(Version)
			os.Exit(0)
		}
		var activePrompt any
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var request struct {
				ID     any             `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				os.Exit(2)
			}
			var result any
			switch request.Method {
			case "initialize":
				result = map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{}}
			case "session/new":
				result = map[string]any{"sessionId": "fixture-session"}
			case "session/set_mode":
				var params struct {
					ModeID string `json:"modeId"`
				}
				if json.Unmarshal(request.Params, &params) != nil || params.ModeID != "work" {
					os.Exit(8)
				}
				result = map[string]any{}
			case "session/prompt":
				var params struct {
					SessionID string `json:"sessionId"`
					Prompt    []struct {
						Text string `json:"text"`
					} `json:"prompt"`
				}
				if json.Unmarshal(request.Params, &params) != nil || params.SessionID != "fixture-session" || len(params.Prompt) != 1 {
					os.Exit(5)
				}
				if params.Prompt[0].Text == "crash" {
					os.Exit(23)
				}
				fixtureSend(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "fixture-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "Hello from fixture"}}}})
				if params.Prompt[0].Text == "permission" {
					activePrompt = request.ID
					fixtureSend(map[string]any{"jsonrpc": "2.0", "id": "fixture-permission", "method": "session/request_permission", "params": map[string]any{"sessionId": "fixture-session", "toolCall": map[string]any{"toolCallId": "tool-1", "title": "Edit file"}, "options": []any{map[string]any{"optionId": "allow", "name": "Allow once", "kind": "allow_once"}, map[string]any{"optionId": "reject", "name": "Reject", "kind": "reject_once"}}}})
					continue
				}
				if params.Prompt[0].Text == "wait" {
					activePrompt = request.ID
					continue
				}
				result = map[string]any{"stopReason": "end_turn"}
			case "":
				if request.ID != "fixture-permission" {
					os.Exit(6)
				}
				fixtureSend(map[string]any{"jsonrpc": "2.0", "id": activePrompt, "result": map[string]any{"stopReason": "end_turn"}})
				activePrompt = nil
				continue
			case "session/cancel":
				fixtureSend(map[string]any{"jsonrpc": "2.0", "id": activePrompt, "result": map[string]any{"stopReason": "cancelled"}})
				activePrompt = nil
				continue
			default:
				os.Exit(3)
			}
			if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
				os.Exit(4)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLaunchACPAndFenceDuplicate(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	provider := &OpenCodeProvider{Binary: binary, WorkDir: t.TempDir(), StateDir: t.TempDir(), Env: []string{"ORKEST_ACP_FIXTURE=1"}}
	config := agentctl.RunConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: agentctl.ModelSelection{Provider: "provider", ID: "model"}, Agent: agentctl.AgentDefinition{Name: "Work", Description: "Work", Instructions: "Work"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process, err := provider.Launch(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	if process.SessionID() != "fixture-session" {
		t.Fatal("missing session")
	}
	configDir := "config-" + fmt.Sprintf("%x", sha256.Sum256([]byte(config.RunID)))
	materialized := filepath.Join(provider.storageRoot(config), configDir, "opencode", "agents", "work.md")
	contents, err := os.ReadFile(materialized)
	if err != nil {
		t.Fatalf("read materialized agent: %v", err)
	}
	for _, expected := range []string{"description: \"Work\"", "mode: primary", "model: \"provider/model\"", "Work"} {
		if !strings.Contains(string(contents), expected) {
			t.Fatalf("materialized agent does not contain %q: %s", expected, contents)
		}
	}
	info, err := os.Stat(materialized)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("agent file permissions are not private: %v", err)
	}
	if _, err := provider.Launch(ctx, config); err == nil {
		t.Fatal("duplicate run launched")
	}
	if err := process.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-process.Done():
	case <-ctx.Done():
		t.Fatal("process did not exit")
	}
}

func fixtureSend(value any) {
	if json.NewEncoder(os.Stdout).Encode(value) != nil {
		os.Exit(7)
	}
}
