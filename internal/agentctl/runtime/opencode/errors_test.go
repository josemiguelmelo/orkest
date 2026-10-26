package opencode

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func TestLaunchRetainsCauseWithoutExposingPaths(t *testing.T) {
	provider := &OpenCodeProvider{Binary: filepath.Join(t.TempDir(), "SECRET"), WorkDir: t.TempDir(), StateDir: t.TempDir()}
	config := agentctl.RunConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: agentctl.ModelSelection{Provider: "provider", ID: "model"}, Agent: agentctl.AgentDefinition{Name: "Work", Description: "Work", Instructions: "Work"}}
	_, err := provider.Launch(context.Background(), config)
	var launch *agentctl.LaunchError
	if !errors.As(err, &launch) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lost launch cause: %v", err)
	}
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(launch.Message, provider.Binary) {
		t.Fatal("exposed executable path")
	}
}

func TestVersionMismatchDoesNotExposeRuntimeOutput(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = verifyVersion(ctx, binary, []string{"ORKEST_ACP_FIXTURE=1", "ORKEST_ACP_WRONG_VERSION=1"})
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("unsafe version error: %v", err)
	}
}

func TestProcessRetainsExitStatus(t *testing.T) {
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
	if process.Err() != nil {
		t.Fatal("live process has exit error")
	}
	runtime := process.(agentctl.RuntimeConversation)
	if _, err := runtime.Prompt(ctx, "crash", agentctl.Observer{}); err == nil {
		t.Fatal("crash succeeded")
	}
	select {
	case <-process.Done():
	case <-ctx.Done():
		t.Fatal("process did not exit")
	}
	var exit *exec.ExitError
	if !errors.As(process.Err(), &exit) || exit.ExitCode() != 23 {
		t.Fatalf("lost exit status: %v", process.Err())
	}
}
