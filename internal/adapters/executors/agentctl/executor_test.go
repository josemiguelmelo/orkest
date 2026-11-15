package agentctl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/checkout"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/sessionpath"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

type checkoutFixture struct{ checkout.Provider }

func (checkoutFixture) PreparePublicTaskCheckout(_ context.Context, _, path, _ string) error {
	return os.MkdirAll(path, 0o700)
}

type planningEngine struct {
	engine.InfrastructureEngine
	workload  engine.Workload
	plan      engine.WorkloadPlan
	startErr  error
	workspace resolvedWorkspace
}

type planInfrastructure struct {
	engine.InfrastructureEngine
	plan     engine.WorkloadPlan
	group    engine.ResourceGroup
	state    engine.GroupState
	stopped  bool
	cleaned  bool
	startErr error
}

func (e *planInfrastructure) StartPlan(_ context.Context, plan engine.WorkloadPlan) (engine.ResourceGroup, error) {
	e.plan = plan
	return e.group, e.startErr
}

func (e *planInfrastructure) Capabilities() engine.Capabilities {
	return engine.Capabilities{Modes: []engine.ExecutionMode{engine.ExecutionModeContainer}, DurableWork: true, DurableState: true, PlatformStorageSetup: true, ExplicitHostBinds: true, CredentialBindings: true}
}

func (*planInfrastructure) ValidatePlan(context.Context, engine.WorkloadPlan) error { return nil }

func (*planInfrastructure) ResolveStorage(_ context.Context, request engine.StorageRequest) ([]engine.StorageBinding, error) {
	bindings := make([]engine.StorageBinding, 0, len(request.Bindings))
	for _, binding := range request.Bindings {
		bindings = append(bindings, engine.StorageBinding{Name: binding.Name, Identity: binding.StorageID, Subpath: binding.Subpath, HostPath: filepath.Join("/tmp", binding.Name)})
	}
	return bindings, nil
}

func (*planInfrastructure) ValidateStorageAccess(context.Context, engine.StorageAccessRequest) error {
	return nil
}
func (*planInfrastructure) Close() error { return nil }

func (e *planInfrastructure) InspectGroup(_ context.Context, ref engine.GroupRef) (engine.GroupObservation, error) {
	if ref.Identity.Owner != e.group.Ref.Identity.Owner || ref.Identity.Generation != e.group.Ref.Identity.Generation {
		return engine.GroupObservation{State: engine.GroupAbsent}, nil
	}
	return engine.GroupObservation{State: e.state, Group: e.group}, nil
}

func (e *planInfrastructure) ListGroups(_ context.Context, selector engine.Selector) ([]engine.ResourceGroup, error) {
	if selector.GroupOwner == "" || selector.GroupOwner == e.group.Ref.Identity.Owner {
		return []engine.ResourceGroup{e.group}, nil
	}
	return nil, nil
}

func (e *planInfrastructure) StopGroup(context.Context, engine.GroupRef) error {
	e.stopped = true
	e.state = engine.GroupStopped
	return nil
}

func (e *planInfrastructure) CleanupGroup(context.Context, engine.GroupRef) error {
	e.cleaned = true
	return nil
}

func TestSharedExecutorPlansHostAndSessionHomeMountsWithoutMutatingRun(t *testing.T) {
	stateRoot := t.TempDir()
	workspace := resolvedWorkspace{Exists: true, PreparationDirectory: t.TempDir(), WorkPath: "/workspace", StatePath: stateRoot}
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
	if len(fake.workload.Mounts) != 3 || fake.workload.Mounts[0] != (engine.Mount{SourceKind: engine.MountSourceHost, Source: "/srv/input", Target: "/mnt/host/input", ReadOnly: true}) || fake.workload.Mounts[1] != (engine.Mount{SourceKind: engine.MountSourceHost, Source: "/srv/settings.xml", Target: home + "/.m2/settings.xml", ReadOnly: true}) || fake.workload.Mounts[2] != (engine.Mount{SourceKind: engine.MountSourceHost, Source: "/srv/output", Target: "/mnt/host/output"}) {
		t.Fatalf("planned host mounts = %#v", fake.workload.Mounts)
	}
	if fake.plan.Workloads[0].Mounts[0].SourceKind != engine.MountSourceHost {
		t.Fatal("generic plan did not declare host mounts")
	}
}

func (e *planningEngine) Capabilities() engine.Capabilities {
	return engine.Capabilities{Modes: []engine.ExecutionMode{engine.ExecutionModeContainer}, DurableWork: true, DurableState: true, PlatformStorageSetup: true, ExplicitHostBinds: true, CredentialBindings: true}
}

func (e *planningEngine) ValidatePlan(context.Context, engine.WorkloadPlan) error { return nil }

func (e *planningEngine) ResolveStorage(_ context.Context, request engine.StorageRequest) ([]engine.StorageBinding, error) {
	bindings := make([]engine.StorageBinding, 0, len(request.Bindings))
	for _, binding := range request.Bindings {
		hostPath := e.workspace.WorkPath
		if binding.Name == "agentctl-state" {
			hostPath = e.workspace.StatePath
		}
		bindings = append(bindings, engine.StorageBinding{Name: binding.Name, Identity: binding.StorageID, Subpath: binding.Subpath, HostPath: hostPath})
	}
	return bindings, nil
}

func (e *planningEngine) ValidateStorageAccess(context.Context, engine.StorageAccessRequest) error {
	return nil
}

func (e *planningEngine) StartPlan(_ context.Context, plan engine.WorkloadPlan) (engine.ResourceGroup, error) {
	e.plan = plan
	e.workload = plan.Workloads[0]
	ref := engine.GroupRef{Identity: engine.GroupIdentity{Owner: plan.Group.Owner, Generation: plan.Group.Generation, InstallationID: "install"}, Provider: "provider-handle"}
	return engine.ResourceGroup{Ref: ref, Endpoints: map[string]string{"agentctl": "http://agentctl.test"}, Components: []engine.Resource{{Ref: engine.ResourceRef{ConcreteID: "provider-component"}, State: engine.ResourceRunning}}}, e.startErr
}

func (e *planningEngine) InspectGroup(context.Context, engine.GroupRef) (engine.GroupObservation, error) {
	return engine.GroupObservation{State: engine.GroupAbsent}, nil
}

func (e *planningEngine) ListGroups(context.Context, engine.Selector) ([]engine.ResourceGroup, error) {
	return nil, nil
}
func (e *planningEngine) StopGroup(context.Context, engine.GroupRef) error    { return nil }
func (e *planningEngine) CleanupGroup(context.Context, engine.GroupRef) error { return nil }
func (e *planningEngine) Close() error                                        { return nil }

func TestSharedExecutorBuildsEngineNeutralLaunchPlan(t *testing.T) {
	workspace := resolvedWorkspace{Exists: true, PreparationDirectory: t.TempDir(), WorkPath: "/workspace", StatePath: "/state"}
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
	if _, err := engine.DecodeGroupRef(process.ID); err != nil {
		t.Fatalf("process ID is not a group handle: %q (%v)", process.ID, err)
	}
	workload := fake.workload
	if fake.plan.Group.Generation != run.RunID || workload.Mode != engine.ExecutionModeContainer {
		t.Fatalf("resource identity/mode = %#v / %v", fake.plan.Group, workload.Mode)
	}
	if workload.Artifact.ContainerImage == "" || fake.workspace != workspace || workload.Restart != engine.RestartOnFailure {
		t.Fatalf("artifact/workspace/restart = %#v / %#v / %v", workload.Artifact, fake.workspace, workload.Restart)
	}
	if !reflect.DeepEqual(workload.Credentials, []engine.CredentialBinding{{Name: "codex-auth", Target: codexContainerAuthPath, ReadOnly: true, EnvironmentName: codexAuthEnvironment}}) {
		t.Fatalf("credential bindings = %#v", workload.Credentials)
	}
	if workload.Environment["TOKEN"] != "overridden" || workload.Environment["EXPLICIT"] != "value" {
		t.Fatalf("environment overrides = %#v", workload.Environment)
	}
	if !reflect.DeepEqual(workload.Arguments[:4], []engine.Argument{{Kind: engine.ArgumentLiteral, Value: "-listen"}, {Kind: engine.ArgumentEndpointAddress, Endpoint: "agentctl"}, {Kind: engine.ArgumentLiteral, Value: "-runtime"}, {Kind: engine.ArgumentLiteral, Value: "codex"}}) {
		t.Fatalf("leading arguments = %#v", workload.Arguments[:4])
	}
}

func TestSharedExecutorLaunchesAndRecoversThroughGenericInfrastructure(t *testing.T) {
	installation := "engine-installation"
	group := engine.ResourceGroup{
		Ref:        engine.GroupRef{Identity: engine.GroupIdentity{InstallationID: installation, Owner: "run-plan", Generation: "run-plan"}, Provider: "provider-handle"},
		Components: []engine.Resource{{Ref: engine.ResourceRef{ConcreteID: "provider-component"}, State: engine.ResourceRunning}},
		Endpoints:  map[string]string{"agentctl": "http://127.0.0.1:43123"},
	}
	infrastructure := &planInfrastructure{group: group, state: engine.GroupActive}
	executor, err := NewSharedAgentCtlExecutor(ExecutorConfig{Engine: infrastructure, Mode: engine.ExecutionModeContainer})
	if err != nil {
		t.Fatal(err)
	}
	run := model.RunConfig{RuntimeConfig: model.RuntimeConfig{
		OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run-plan", TaskSessionID: "session",
		Runtime: "opencode", Model: model.ModelSelection{Provider: "provider", ID: "model"}, Agent: model.AgentDefinition{Name: "worker", Instructions: "test"},
	}}
	process, err := executor.StartAgentctl(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := engine.DecodeGroupRef(process.ID)
	if err != nil || ref != group.Ref {
		t.Fatalf("generic process reference = %#v, %v", ref, err)
	}
	if process.Endpoint != group.Endpoints["agentctl"] {
		t.Fatalf("process endpoint = %q", process.Endpoint)
	}
	plan := infrastructure.plan
	if plan.Group.InstallationID != "" || plan.Group.Owner != run.RunID || plan.Group.Generation != run.RunID {
		t.Fatalf("executor leaked engine installation into plan identity: %#v", plan.Group)
	}
	if len(plan.Storage) != 2 || plan.Storage[0].StorageID == "" || plan.Storage[0].Subpath != "work" || plan.Storage[1].Subpath != "agentctl-state" {
		t.Fatalf("generic storage attachments = %#v", plan.Storage)
	}
	if running, runningErr := executor.AgentctlRunning(context.Background(), process.ID); runningErr != nil || !running {
		t.Fatalf("generic process inspection = %v, %v", running, runningErr)
	}
	if err = executor.TerminateAgentctl(context.Background(), process.ID); err != nil || !infrastructure.stopped || !infrastructure.cleaned {
		t.Fatalf("generic process termination: err=%v stopped=%v cleaned=%v", err, infrastructure.stopped, infrastructure.cleaned)
	}
}

func TestGenericAgentPlanProtectsTaskStorageWithWritableCheckoutChildren(t *testing.T) {
	workPath := t.TempDir()
	statePath := t.TempDir()
	workspace := resolvedWorkspace{Exists: true, PreparationDirectory: workPath, WorkPath: workPath, StatePath: statePath}
	fake := &planningEngine{workspace: workspace}
	executor, err := NewSharedAgentCtlExecutor(ExecutorConfig{
		Engine: fake, Mode: engine.ExecutionModeContainer,
		Repositories: WorkspaceConfig{Repositories: checkout.Preparer{Providers: map[string]checkout.Provider{"github": checkoutFixture{}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	run := model.RunConfig{RuntimeConfig: model.RuntimeConfig{
		OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run-storage", TaskSessionID: "session",
		Runtime: "opencode", Model: model.ModelSelection{Provider: "provider", ID: "model"}, Agent: model.AgentDefinition{Name: "worker", Instructions: "test"},
	}}
	run.Repositories = []model.WorkspaceRepository{{ID: "repository", ProviderName: "github", URL: "https://github.com/example/repository.git"}}
	if _, err = executor.StartAgentctl(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	plan := fake.plan
	if len(plan.Storage) != 2 || !plan.Storage[0].ReadOnly || plan.Storage[1].ReadOnly {
		t.Fatalf("task storage attachments = %#v", plan.Storage)
	}
	if len(plan.Workloads) != 1 || len(plan.Workloads[0].Mounts) != 2 {
		t.Fatalf("protected checkout mounts = %#v", plan.Workloads)
	}
	parent, checkoutMount := plan.Workloads[0].Mounts[0], plan.Workloads[0].Mounts[1]
	canonicalWorkPath, resolveErr := filepath.EvalSymlinks(workPath)
	wantSource := filepath.Join(canonicalWorkPath, "repositories", "github", "repository")
	if resolveErr != nil || parent.Source != filepath.Join(canonicalWorkPath, "repositories") || parent.Target != "/workspace/repositories" || !parent.ReadOnly || checkoutMount.Target != "/workspace/repositories/github/repository" || checkoutMount.ReadOnly || checkoutMount.Source != wantSource {
		t.Fatalf("protected mount ancestry = %#v", plan.Workloads[0].Mounts)
	}
}

func TestSharedExecutorPreservesResourceReferenceOnPartialStart(t *testing.T) {
	startErr := errors.New("runtime response lost")
	fake := &planningEngine{workspace: resolvedWorkspace{Exists: true, PreparationDirectory: t.TempDir(), WorkPath: filepath.Join(t.TempDir(), "work"), StatePath: filepath.Join(t.TempDir(), "state")}, startErr: startErr}
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
	if _, decodeErr := engine.DecodeGroupRef(process.ID); decodeErr != nil {
		t.Fatalf("partial process ID is not a generic group handle: %q (%v)", process.ID, decodeErr)
	}
}
