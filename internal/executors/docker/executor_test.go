package docker

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	mobyclient "github.com/moby/moby/client"
)

func TestStartAgentctlCreatesIsolatedContainerAndReturnsLoopbackEndpoint(t *testing.T) {
	engine := &fakeEngine{containerID: "container-id", hostPort: "49152", running: true}
	executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir(), Image: "orkest/opencode:test", ContainerUser: "1000:1000"}, engine)
	if err != nil {
		t.Fatal(err)
	}

	process, err := executor.StartAgentctl(context.Background(), runConfig())
	if err != nil {
		t.Fatal(err)
	}
	if process.ID != "container-id" || process.Endpoint != "http://127.0.0.1:49152" {
		t.Fatalf("unexpected process: %+v", process)
	}
	if engine.startedID != process.ID {
		t.Fatalf("started ID = %q, want %q", engine.startedID, process.ID)
	}
	if engine.created.Name != containerName("run") || !strings.HasPrefix(engine.created.Name, "orkest-agentctl-") {
		t.Fatalf("container name = %q", engine.created.Name)
	}
	if engine.created.Config.Image != "orkest/opencode:test" || engine.created.Config.User != "1000:1000" {
		t.Fatalf("container config = %+v", engine.created.Config)
	}
	if len(engine.created.Config.Env) != 0 {
		t.Fatalf("unexpected environment passed to container: %v", engine.created.Config.Env)
	}
	if strings.Join(engine.created.Config.Cmd, " ") != "-listen 0.0.0.0:8081 -work-dir /workspace -state-dir /agentctl-state" {
		t.Fatalf("container command = %v", engine.created.Config.Cmd)
	}
	if got := engine.created.HostConfig.RestartPolicy.Name; got != container.RestartPolicyDisabled {
		t.Fatalf("restart policy = %s", got)
	}
	if got := engine.created.HostConfig.PortBindings[network.MustParsePort("8081/tcp")]; len(got) != 1 || got[0].HostIP.String() != "127.0.0.1" || got[0].HostPort != "" {
		t.Fatalf("port binding = %+v", got)
	}
	if len(engine.created.HostConfig.Mounts) != 2 {
		t.Fatalf("mounts = %+v", engine.created.HostConfig.Mounts)
	}
	expectedMounts := map[string]bool{workMountPath: false, stateMountPath: false}
	for _, mount := range engine.created.HostConfig.Mounts {
		if _, ok := expectedMounts[mount.Target]; !ok {
			t.Fatalf("unexpected mount target: %+v", mount)
		}
		expectedMounts[mount.Target] = true
		if mount.Type != "bind" || mount.ReadOnly {
			t.Fatalf("mount is not a writable bind: %+v", mount)
		}
		info, statErr := os.Stat(mount.Source)
		if statErr != nil || !info.IsDir() {
			t.Fatalf("mount source %q is not a directory: %v", mount.Source, statErr)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("mount source permissions = %o", info.Mode().Perm())
		}
	}
	if !expectedMounts[workMountPath] || !expectedMounts[stateMountPath] {
		t.Fatalf("required task mount missing: %v", expectedMounts)
	}
	if _, ok := engine.created.Config.Labels["orkest.managed"]; !ok {
		t.Fatalf("missing managed label: %v", engine.created.Config.Labels)
	}
}

func TestStartAgentctlRemovesContainerWhenStartFails(t *testing.T) {
	engine := &fakeEngine{containerID: "container-id", startErr: errors.New("docker start failed")}
	executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir(), ContainerUser: "1000:1000"}, engine)
	if err != nil {
		t.Fatal(err)
	}
	process, err := executor.StartAgentctl(context.Background(), runConfig())
	if err == nil {
		t.Fatal("expected start failure")
	}
	if process.ID != "container-id" {
		t.Fatalf("process ID = %q", process.ID)
	}
	if engine.removedID != "container-id" {
		t.Fatalf("removed ID = %q", engine.removedID)
	}
}

func TestStartAgentctlReturnsIdentityWhenCleanupFails(t *testing.T) {
	engine := &fakeEngine{containerID: "container-id", startErr: errors.New("docker start failed"), removeErr: errors.New("cleanup failed")}
	executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir(), ContainerUser: "1000:1000"}, engine)
	if err != nil {
		t.Fatal(err)
	}
	process, err := executor.StartAgentctl(context.Background(), runConfig())
	if err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("error = %v", err)
	}
	if process.ID != "container-id" {
		t.Fatalf("process ID = %q", process.ID)
	}
}

func TestStartAgentctlDoesNotRemoveContainerWhenStartOutcomeIsUncertain(t *testing.T) {
	engine := &fakeEngine{containerID: "container-id", startErr: errors.New("docker start timed out"), inspectErr: errors.New("inspect unavailable")}
	executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir(), ContainerUser: "1000:1000"}, engine)
	if err != nil {
		t.Fatal(err)
	}
	process, err := executor.StartAgentctl(context.Background(), runConfig())
	if err == nil {
		t.Fatal("expected uncertain start failure")
	}
	if process.ID != "container-id" {
		t.Fatalf("process ID = %q", process.ID)
	}
	if engine.removedID != "" {
		t.Fatalf("uncertain container was removed: %q", engine.removedID)
	}
}

func TestStartAgentctlDoesNotRemoveContainerWhenStateIsMissing(t *testing.T) {
	engine := &fakeEngine{containerID: "container-id", startErr: errors.New("docker start timed out"), stateMissing: true}
	executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir(), ContainerUser: "1000:1000"}, engine)
	if err != nil {
		t.Fatal(err)
	}
	process, err := executor.StartAgentctl(context.Background(), runConfig())
	if err == nil {
		t.Fatal("expected uncertain state failure")
	}
	if process.ID != "container-id" || engine.removedID != "" {
		t.Fatalf("process = %+v; removed = %q", process, engine.removedID)
	}
}

func TestStartAgentctlAcceptsContainerRunningAfterStartTimeout(t *testing.T) {
	engine := &fakeEngine{containerID: "container-id", hostPort: "49152", running: true, startErr: errors.New("docker start timed out")}
	executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir(), ContainerUser: "1000:1000"}, engine)
	if err != nil {
		t.Fatal(err)
	}
	process, err := executor.StartAgentctl(context.Background(), runConfig())
	if err != nil {
		t.Fatal(err)
	}
	if process.ID != "container-id" || process.Endpoint != "http://127.0.0.1:49152" {
		t.Fatalf("unexpected process: %+v", process)
	}
	if engine.removedID != "" {
		t.Fatalf("running container was removed: %q", engine.removedID)
	}
}

func TestStartAgentctlRetainsIdentityWhenInspectFailsAfterStart(t *testing.T) {
	engine := &fakeEngine{containerID: "container-id", running: true, inspectErr: errors.New("inspect unavailable")}
	executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir(), ContainerUser: "1000:1000"}, engine)
	if err != nil {
		t.Fatal(err)
	}
	process, err := executor.StartAgentctl(context.Background(), runConfig())
	if err == nil {
		t.Fatal("expected inspect failure")
	}
	if process.ID != "container-id" || engine.removedID != "" {
		t.Fatalf("process = %+v; removed = %q", process, engine.removedID)
	}
}

func TestTaskDirectoriesAreSharedAcrossRunsButIsolatedAcrossTasks(t *testing.T) {
	executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir(), ContainerUser: "1000:1000"}, &fakeEngine{})
	if err != nil {
		t.Fatal(err)
	}
	first := runConfig()
	workOne, stateOne, err := executor.taskDirectories(first)
	if err != nil {
		t.Fatal(err)
	}
	first.RunID = "another-run"
	workTwo, stateTwo, err := executor.taskDirectories(first)
	if err != nil {
		t.Fatal(err)
	}
	if workOne != workTwo || stateOne != stateTwo {
		t.Fatalf("paths changed across same-task runs: %q %q / %q %q", workOne, stateOne, workTwo, stateTwo)
	}
	first.TaskID = "other-task"
	workThree, stateThree, err := executor.taskDirectories(first)
	if err != nil {
		t.Fatal(err)
	}
	if workOne == workThree || stateOne == stateThree {
		t.Fatalf("task paths are shared: %q %q", workOne, workThree)
	}
	if !filepath.IsAbs(workOne) || !filepath.IsAbs(stateOne) {
		t.Fatal("mount paths are not absolute")
	}
}

func TestNewWithClientRejectsInvalidSettings(t *testing.T) {
	for _, config := range []Config{{DataRoot: t.TempDir(), ContainerUser: "1000"}, {DataRoot: t.TempDir(), AgentctlPort: 65536, ContainerUser: "1000:1000"}} {
		if _, err := NewWithClient(context.Background(), config, &fakeEngine{}); err == nil {
			t.Fatalf("expected invalid config error for %+v", config)
		}
	}
	if _, err := NewWithClient(context.Background(), Config{Image: "image", ContainerUser: "1000:1000"}, &fakeEngine{}); err == nil {
		t.Fatal("expected missing data root error")
	}
}

func runConfig() agentctl.RunConfig {
	return agentctl.RunConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: agentctl.ModelSelection{Provider: "provider", ID: "model"}, SystemPrompt: "work"}
}

type fakeEngine struct {
	created      mobyclient.ContainerCreateOptions
	containerID  string
	hostPort     string
	startedID    string
	removedID    string
	startErr     error
	removeErr    error
	inspectErr   error
	stateMissing bool
	running      bool
}

func (f *fakeEngine) ContainerCreate(_ context.Context, options mobyclient.ContainerCreateOptions) (mobyclient.ContainerCreateResult, error) {
	f.created = options
	return mobyclient.ContainerCreateResult{ID: f.containerID}, nil
}

func (f *fakeEngine) ContainerStart(_ context.Context, id string, _ mobyclient.ContainerStartOptions) (mobyclient.ContainerStartResult, error) {
	f.startedID = id
	return mobyclient.ContainerStartResult{}, f.startErr
}

func (f *fakeEngine) ContainerInspect(_ context.Context, _ string, _ mobyclient.ContainerInspectOptions) (mobyclient.ContainerInspectResult, error) {
	if f.inspectErr != nil {
		return mobyclient.ContainerInspectResult{}, f.inspectErr
	}
	var state *container.State
	if !f.stateMissing {
		state = &container.State{Running: f.running}
	}
	return mobyclient.ContainerInspectResult{Container: container.InspectResponse{State: state, NetworkSettings: &container.NetworkSettings{Ports: network.PortMap{network.MustParsePort("8081/tcp"): {{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: f.hostPort}}}}}}, nil
}

func (f *fakeEngine) ContainerRemove(_ context.Context, id string, _ mobyclient.ContainerRemoveOptions) (mobyclient.ContainerRemoveResult, error) {
	f.removedID = id
	return mobyclient.ContainerRemoveResult{}, f.removeErr
}
func (*fakeEngine) Close() error { return nil }

var (
	_ orchestration.ProcessExecutor = (*Executor)(nil)
	_ EngineClient                  = (*fakeEngine)(nil)
)
