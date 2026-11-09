//go:build unix

package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

func TestStartAgentctlPassesHostPathToRuntime(t *testing.T) {
	hostPath := strings.Join([]string{os.Getenv("PATH"), t.TempDir(), "/bin", "/usr/bin"}, string(os.PathListSeparator))
	t.Setenv("PATH", hostPath)
	capturePath := filepath.Join(t.TempDir(), "agentctl-invocation")
	t.Setenv("CAPTURE_FILE", capturePath)

	binary := filepath.Join(t.TempDir(), "agentctl")
	script := "#!/bin/sh\nprintf 'PATH=%s\\n' \"$PATH\" > \"$CAPTURE_FILE\"\nfor arg do printf 'ARG=%s\\n' \"$arg\" >> \"$CAPTURE_FILE\"; done\nexec /bin/sleep 30\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	executor, err := New(Config{
		DataRoot:       t.TempDir(),
		AgentctlBinary: binary,
		ProviderEnv:    []string{"CAPTURE_FILE"},
	})
	if err != nil {
		t.Fatal(err)
	}

	process, err := executor.StartAgentctl(context.Background(), orchestrationmodel.RunConfig{
		RuntimeConfig: orchestrationmodel.RuntimeConfig{
			OrganizationID: "org",
			WorkspaceID:    "workspace",
			TaskID:         "task",
			RunID:          "run",
			Runtime:        "opencode",
			Model:          orchestrationmodel.ModelSelection{Provider: "provider", ID: "model"},
			Agent:          orchestrationmodel.AgentDefinition{Name: "Work", Instructions: "work"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := executor.TerminateAgentctl(ctx, process.ID); err != nil {
			t.Errorf("terminate fake agentctl: %v", err)
		}
	})

	deadline := time.Now().Add(2 * time.Second)
	var invocation string
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(capturePath)
		if readErr == nil {
			invocation = string(data)
			if strings.Contains(invocation, "ARG=PATH\n") {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if invocation == "" {
		t.Fatal("fake agentctl did not capture its environment and arguments")
	}
	if !strings.Contains(invocation, "PATH="+hostPath+"\n") {
		t.Fatalf("runtime PATH was not inherited; invocation: %q", invocation)
	}
	if !strings.Contains(invocation, "ARG=-env\nARG=PATH\n") {
		t.Fatalf("PATH was not granted to the runtime; invocation: %q", invocation)
	}
}
