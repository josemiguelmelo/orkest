package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/contracts/mcptransport"

	sharedacp "github.com/josemiguelmelo/orkest/internal/adapters/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"
	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The same test binary serves as a real stdio MCP subprocess. It never needs a model.
func TestMain(m *testing.M) {
	if os.Getenv("ORKEST_TEST_STDIO_CHILD") == "1" {
		os.Exit(runStdioMCP())
	}
	os.Exit(m.Run())
}

func runStdioMCP() int {
	marker := os.Getenv("ORKEST_TEST_STDIO_MARKER")
	if marker == "" || len(os.Args) != 2 || os.Args[1] != "literal argument" || os.Getenv("MCP_TOKEN") == "" {
		return 2
	}
	record := func(kind string) {
		file, err := os.OpenFile(marker, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintln(file, kind)
		_ = file.Close()
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "Orkest stdio integration", Version: "1.0.0"}, &mcp.ServerOptions{InitializedHandler: func(context.Context, *mcp.InitializedRequest) { record("initialized") }})
	server.AddTool(&mcp.Tool{Name: "stdio_probe", Description: "Return a deterministic test marker", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "stdio-connected"}}}, nil
	})
	// Diagnostics use stderr; stdout is reserved for the MCP protocol.
	_, _ = fmt.Fprintln(os.Stderr, "Orkest stdio MCP fixture starting")
	err := server.Run(context.Background(), &mcp.StdioTransport{})
	record("stopped")
	if err != nil {
		return 3
	}
	return 0
}

func TestOpenCodeStdioMCPHandshake(t *testing.T) {
	binary := os.Getenv("ORKEST_OPENCODE_BINARY")
	if binary == "" {
		t.Fatal("set ORKEST_OPENCODE_BINARY to the pinned OpenCode executable")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	work, state := t.TempDir(), t.TempDir()
	marker := filepath.Join(work, "stdio-lifecycle")
	config := configmodel.RuntimeConfig{OrganizationID: "stdio-org", WorkspaceID: "stdio-workspace", TaskID: "stdio-task", TaskSessionID: "stdio-session", RunID: "first", Runtime: "opencode", Agent: configmodel.AgentDefinition{Name: "Integration", Instructions: "You are a test agent."}, Model: configmodel.ModelSelection{Provider: "opencode", ID: "big-pickle"}, MCPServers: []configmodel.MCPServer{{ID: "stdio", Name: "stdio", Transport: mcptransport.Stdio, Command: executable, Args: []string{"literal argument"}, Environment: map[string]string{"ORKEST_TEST_STDIO_CHILD": "1", "ORKEST_TEST_STDIO_MARKER": marker, "MCP_TOKEN": "first-test-token"}}}}
	host := runtimehost.New(opencode.Profile{Binary: binary}, sharedacp.Factory{}, runtimehost.Environment{WorkDir: work, StateDir: state, GrantedEnv: []string{"PATH=" + os.Getenv("PATH")}})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	first, err := host.Launch(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	waitStdioMarker(t, ctx, marker, "initialized", 1)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	waitStdioMarker(t, ctx, marker, "stopped", 1)
	config.RunID = "second"
	config.MCPServers[0].Environment["MCP_TOKEN"] = "rotated-test-token"
	restored, err := host.Restore(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if restored.SessionID() != first.SessionID() {
		t.Fatal("stdio restoration replaced the native session")
	}
	waitStdioMarker(t, ctx, marker, "initialized", 2)
	if err := restored.Close(); err != nil {
		t.Fatal(err)
	}
	waitStdioMarker(t, ctx, marker, "stopped", 2)
}

func waitStdioMarker(t *testing.T, ctx context.Context, marker, kind string, count int) {
	t.Helper()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		data, _ := os.ReadFile(marker)
		if strings.Count(string(data), kind+"\n") >= count {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("stdio lifecycle check timed out", kind)
		case <-timeout.C:
			t.Fatal("stdio lifecycle marker missing", kind)
		case <-tick.C:
		}
	}
}
