package agentctl

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/sessionpath"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

type planningEngine struct {
	engine.ExecutorEngine
	spec      engine.LaunchSpec
	startErr  error
	workspace engine.Workspace
}

func TestSharedExecutorPlansHostAndSessionHomeMountsWithoutMutatingRun(t *testing.T) {
	stateRoot := t.TempDir()
	workspace := engine.Workspace{Exists: true, PreparationDirectory: t.TempDir(), WorkPath: "/workspace", StatePath: stateRoot}
	fake := &planningEngine{workspace: workspace}
	executor, err := NewSharedAgentCtlExecutor(ExecutorConfig{Engine: fake, Mode: engine.ExecutionModeContainer})
	if err != nil {
		t.Fatal(err)
	}
	run := model.RunConfig{RuntimeConfig: model.RuntimeConfig{
		OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run-mounts", TaskSessionID: "session-one",
		Runtime: "opencode", Model: model.ModelSelection{Provider: "provider", ID: "model"}, Agent: model.AgentDefinition{Name: "work", Instructions: "test"},
	}}
	run.HostMounts = []mountmodel.Mount{
		{Source: "/srv/input", Target: "/mnt/host/input", ReadOnly: true},
		{Source: "/srv/settings.xml", Target: "~/.m2/settings.xml", ReadOnly: true},
		{Source: "/srv/output", Target: "/mnt/host/output"},
	}
	want := run.Clone()
	if _, err := executor.StartAgentctl(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(run.HostMounts, want.HostMounts) {
		t.Fatal("planning mutated persisted host mount selections")
	}
	home := filepath.ToSlash(sessionpath.Home("/agentctl-state", "org", "workspace", "task", "session-one"))
	if len(fake.spec.Mounts) != 3 || fake.spec.Mounts[0] != (engine.Mount{SourceKind: engine.MountSourceHost, Source: "/srv/input", Target: "/mnt/host/input", ReadOnly: true}) || fake.spec.Mounts[1] != (engine.Mount{SourceKind: engine.MountSourceHost, Source: "/srv/settings.xml", Target: home + "/.m2/settings.xml", ReadOnly: true}) || fake.spec.Mounts[2] != (engine.Mount{SourceKind: engine.MountSourceHost, Source: "/srv/output", Target: "/mnt/host/output"}) {
		t.Fatalf("planned host mounts = %#v", fake.spec.Mounts)
	}
	if !fake.spec.Requirements.ExplicitHostBinds {
		t.Fatal("launch requirements did not declare explicit host mounts")
	}
}

func (e *planningEngine) Capabilities() engine.Capabilities {
	return engine.Capabilities{Modes: []engine.ExecutionMode{engine.ExecutionModeContainer}, DurableWork: true, DurableState: true, PlatformWorkspaceSetup: true, ExplicitHostBinds: true, CredentialBindings: true, RestartModes: true}
}

func (e *planningEngine) Validate(context.Context, engine.Requirements) error { return nil }

func (e *planningEngine) ResolveWorkspace(context.Context, engine.WorkspaceRequest) (engine.Workspace, error) {
	return e.workspace, nil
}

func (e *planningEngine) Start(_ context.Context, spec engine.LaunchSpec) (engine.Resource, error) {
	e.spec = spec
	return engine.Resource{Ref: engine.ResourceRef{Key: spec.Key, ConcreteID: "provisional-resource"}, Endpoint: "http://agentctl.test"}, e.startErr
}

func TestSharedExecutorBuildsEngineNeutralLaunchPlan(t *testing.T) {
	workspace := engine.Workspace{Exists: true, PreparationDirectory: t.TempDir(), WorkPath: "/workspace", StatePath: "/state"}
	fake := &planningEngine{workspace: workspace}
	executor, err := NewSharedAgentCtlExecutor(ExecutorConfig{Engine: fake, Mode: engine.ExecutionModeContainer, Images: nil, ProviderEnv: []string{"TOKEN"}, Environment: map[string]string{"TOKEN": "overridden", "EXPLICIT": "value"}, CodexAuthFile: "/host/codex-auth.json"})
	if err != nil {
		t.Fatal(err)
	}
	run := model.RunConfig{RuntimeConfig: model.RuntimeConfig{
		OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run-7", TaskSessionID: "session",
		Runtime: "codex", Model: model.ModelSelection{Provider: "openai", ID: "gpt-5-codex"},
		Agent: model.AgentDefinition{Name: "work", Instructions: "do work"},
	}}
	process, err := executor.StartAgentctl(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if process.ID != "provisional-resource" {
		t.Fatalf("process ID = %q", process.ID)
	}
	spec := fake.spec
	if spec.Key.ExecutionID != run.RunID || spec.Key.Qualifier != string(run.Runtime) || spec.Mode != engine.ExecutionModeContainer {
		t.Fatalf("resource identity/mode = %#v / %v", spec.Key, spec.Mode)
	}
	if spec.Artifact.ContainerImage == "" || spec.Workspace != workspace || spec.Restart != engine.RestartOnFailure {
		t.Fatalf("artifact/workspace/restart = %#v / %#v / %v", spec.Artifact, spec.Workspace, spec.Restart)
	}
	if !reflect.DeepEqual(spec.CredentialBindings, []engine.CredentialBinding{{Name: "codex-auth", Target: codexContainerAuthPath, ReadOnly: true}}) {
		t.Fatalf("credential bindings = %#v", spec.CredentialBindings)
	}
	if spec.Environment["TOKEN"] != "overridden" || spec.Environment["EXPLICIT"] != "value" {
		t.Fatalf("environment overrides = %#v", spec.Environment)
	}
	if !reflect.DeepEqual(spec.Arguments[:4], []engine.Argument{{Kind: engine.ArgumentLiteral, Value: "-listen"}, {Kind: engine.ArgumentEndpointAddress, Endpoint: "agentctl"}, {Kind: engine.ArgumentLiteral, Value: "-runtime"}, {Kind: engine.ArgumentLiteral, Value: "codex"}}) {
		t.Fatalf("leading arguments = %#v", spec.Arguments[:4])
	}
}

func TestSharedExecutorPreservesResourceReferenceOnPartialStart(t *testing.T) {
	startErr := errors.New("runtime response lost")
	fake := &planningEngine{workspace: engine.Workspace{Exists: true, PreparationDirectory: t.TempDir(), WorkPath: filepath.Join(t.TempDir(), "work"), StatePath: filepath.Join(t.TempDir(), "state")}, startErr: startErr}
	executor, err := NewSharedAgentCtlExecutor(ExecutorConfig{Engine: fake, Mode: engine.ExecutionModeContainer})
	if err != nil {
		t.Fatal(err)
	}
	run := model.RunConfig{RuntimeConfig: model.RuntimeConfig{
		OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run-uncertain", Runtime: "opencode",
		Model: model.ModelSelection{Provider: "provider", ID: "model"}, Agent: model.AgentDefinition{Name: "work", Instructions: "do work"},
	}}
	process, err := executor.StartAgentctl(context.Background(), run)
	if !errors.Is(err, startErr) {
		t.Fatalf("StartAgentctl error = %v", err)
	}
	if process.ID != "provisional-resource" {
		t.Fatalf("partial process ID = %q", process.ID)
	}
}
