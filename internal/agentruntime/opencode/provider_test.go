package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func TestMain(m *testing.M) {
	if os.Getenv("ORKEST_ACP_FIXTURE") == "1" {
		if os.Args[1] == "--version" {
			fmt.Println(Version)
			os.Exit(0)
		}
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
				result = map[string]any{}
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
	provider := &Provider{Binary: binary, WorkDir: t.TempDir(), StateDir: t.TempDir(), Env: []string{"ORKEST_ACP_FIXTURE=1"}}
	config := agentctl.Configuration{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: agentctl.Model{Provider: "provider", ID: "model"}, SystemPrompt: "Work"}
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
