//go:build unix

package agentctl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	enginecontract "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	localengine "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine/local"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

func TestSharedAgentCtlExecutorRunsAndControlsNativeWorkload(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "agentctl")
	capture := filepath.Join(root, "invocation")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE_FILE\"\ntrap 'exit 0' INT TERM\nwhile :; do /bin/sleep 1; done\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	localEngine, err := localengine.New(localengine.Config{DataRoot: filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewSharedAgentCtlExecutor(ExecutorConfig{
		Engine: localEngine, Mode: enginecontract.ExecutionModeNative, AgentctlBinary: binary,
		Environment: map[string]string{"CAPTURE_FILE": capture},
		ProviderEnv: []string{"CAPTURE_FILE"},
	})
	if err != nil {
		t.Fatal(err)
	}
	run := model.RunConfig{RuntimeConfig: model.RuntimeConfig{
		OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run-1",
		Runtime: "opencode", Model: model.ModelSelection{Provider: "provider", ID: "model"},
		Agent: model.AgentDefinition{Name: "work", Instructions: "do work"},
	}}
	process, err := executor.StartAgentctl(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if stopErr := executor.TerminateAgentctl(ctx, process.ID); stopErr != nil {
			t.Errorf("terminate local workload: %v", stopErr)
		}
	})
	if !strings.HasPrefix(process.Endpoint, "http://127.0.0.1:") {
		t.Fatalf("native endpoint = %q", process.Endpoint)
	}
	deadline := time.Now().Add(2 * time.Second)
	var args string
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(capture)
		if readErr == nil {
			args = string(data)
			if strings.Contains(args, "-run-id\nrun-1\n") && strings.Contains(args, "-env\nPATH\n") {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(args, "-run-id\nrun-1\n") || !strings.Contains(args, "-env\nPATH\n") {
		t.Fatalf("native worker args = %q", args)
	}
	running, err := executor.AgentctlRunningByRunID(context.Background(), run.RunID)
	if err != nil || !running {
		t.Fatalf("AgentctlRunningByRunID() = %v, %v", running, err)
	}
	if err = executor.TerminateAgentctlByRunID(context.Background(), run.RunID); err != nil {
		t.Fatal(err)
	}
	running, err = executor.AgentctlRunningByRunID(context.Background(), run.RunID)
	if err != nil || running {
		t.Fatalf("AgentctlRunningByRunID() after stop = %v, %v", running, err)
	}
}
