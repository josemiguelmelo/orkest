package runtimehost_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode/opencodetest"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"
	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestLaunchRetainsCauseWithoutExposingPaths(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "SECRET")
	environment := runtimehost.Environment{WorkDir: t.TempDir(), StateDir: t.TempDir()}
	config := runconfig.RunConfig{RuntimeConfig: runconfig.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: runconfig.ModelSelection{Provider: "provider", ID: "model"}, Agent: runconfig.AgentDefinition{Name: "Work", Description: "Work", Instructions: "Work"}}}
	_, err := opencodetest.NewHost(binary, environment).Launch(context.Background(), config.ForRuntime())
	var launch *agentruntime.LaunchError
	if !errors.As(err, &launch) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lost launch cause: %v", err)
	}
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(launch.Message, binary) {
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
	environment := runtimehost.Environment{WorkDir: t.TempDir(), StateDir: t.TempDir(), GrantedEnv: []string{"ORKEST_ACP_FIXTURE=1", "ORKEST_ACP_WRONG_VERSION=1"}}
	config := runconfig.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: runconfig.ModelSelection{Provider: "provider", ID: "model"}, Agent: runconfig.AgentDefinition{Name: "Work", Description: "Work", Instructions: "Work"}}
	_, err = opencodetest.NewHost(binary, environment).Launch(ctx, config)
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("unsafe version error: %v", err)
	}
}

func TestProcessRetainsExitStatus(t *testing.T) {
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
	if process.Err() != nil {
		t.Fatal("live process has exit error")
	}
	runtime := process
	if _, err := runtime.Prompt(ctx, interaction.Command{ID: "test-command", Text: "crash"}, agentruntime.PromptObserver{}); err == nil {
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
