// These tests exercise OpenCode profile + ACP + host wiring. They are fixture
// tests, not qualification against the real OpenCode executable.
package runtime_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode/opencodetest"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtimehost"
)

func TestLaunchACPAndFenceDuplicate(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environment := runtimehost.Environment{WorkDir: t.TempDir(), StateDir: t.TempDir(), GrantedEnv: []string{"ORKEST_ACP_FIXTURE=1"}}
	config := runconfig.RunConfig{RuntimeConfig: runconfig.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: runconfig.ModelSelection{Provider: "provider", ID: "model"}, Agent: runconfig.AgentDefinition{Name: "Work", Description: "Work", Instructions: "Work"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process, err := opencodetest.NewHost(binary, environment).Launch(ctx, config.ForRuntime())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	if process.SessionID() != "fixture-session" {
		t.Fatal("missing session")
	}
	configDir := "config-" + fmt.Sprintf("%x", sha256.Sum256([]byte(config.RunID)))
	matches, err := filepath.Glob(filepath.Join(environment.StateDir, "*", configDir, "opencode", "agents", "work.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected one materialized agent definition: %v %v", matches, err)
	}
	materialized := matches[0]
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
	if _, err := opencodetest.NewHost(binary, environment).Launch(ctx, config.ForRuntime()); err == nil {
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
