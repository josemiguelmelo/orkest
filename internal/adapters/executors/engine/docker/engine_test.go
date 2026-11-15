package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	client "github.com/moby/moby/client"
)

type fakeClient struct {
	Client
	created    client.ContainerCreateOptions
	resource   container.InspectResponse
	running    bool
	createErr  error
	startErr   error
	inspectErr error
	removed    bool
	closed     int
}

func (f *fakeClient) ContainerCreate(_ context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	f.created = options
	if f.createErr != nil {
		return client.ContainerCreateResult{}, f.createErr
	}
	f.resource = container.InspectResponse{
		ID: "docker-id", Name: "/" + options.Name, Config: options.Config, HostConfig: options.HostConfig,
		State: &container.State{}, NetworkSettings: &container.NetworkSettings{Ports: network.PortMap{
			network.MustParsePort("8081/tcp"): {{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "49152"}},
		}},
	}
	return client.ContainerCreateResult{ID: "docker-id"}, nil
}

func (f *fakeClient) ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
	f.running = true
	if f.resource.State != nil {
		f.resource.State.Running = true
	}
	return client.ContainerStartResult{}, f.startErr
}

func (f *fakeClient) ContainerStop(context.Context, string, client.ContainerStopOptions) (client.ContainerStopResult, error) {
	f.running = false
	if f.resource.State != nil {
		f.resource.State.Running = false
	}
	return client.ContainerStopResult{}, nil
}

func (f *fakeClient) ContainerInspect(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if f.inspectErr != nil {
		return client.ContainerInspectResult{}, f.inspectErr
	}
	if f.removed || f.resource.ID == "" {
		return client.ContainerInspectResult{}, errdefs.ErrNotFound
	}
	if id != f.resource.ID && strings.TrimPrefix(f.resource.Name, "/") != id {
		return client.ContainerInspectResult{}, errdefs.ErrNotFound
	}
	return client.ContainerInspectResult{Container: f.resource}, nil
}

func (f *fakeClient) ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.removed = true
	return client.ContainerRemoveResult{}, nil
}

func (f *fakeClient) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	if f.removed || f.resource.ID == "" {
		return client.ContainerListResult{}, nil
	}
	return client.ContainerListResult{Items: []container.Summary{{ID: f.resource.ID}}}, nil
}

func (*fakeClient) ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	return io.NopCloser(&emptyReader{}), nil
}

func (f *fakeClient) Close() error { f.closed++; return nil }

type emptyReader struct{}

func (*emptyReader) Read([]byte) (int, error) { return 0, io.EOF }

func newFixture(t *testing.T, fake *fakeClient) (*ExecutorEngine, engine.Workspace, engine.ResourceKey) {
	t.Helper()
	instance, err := New(Config{DataRoot: t.TempDir(), ContainerUser: "1000:1000", Client: fake})
	if err != nil {
		t.Fatal(err)
	}
	scope := engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	workspace, err := instance.ResolveWorkspace(context.Background(), engine.WorkspaceRequest{Scope: scope, Intent: engine.WorkspaceEnsureForAgent})
	if err != nil {
		t.Fatal(err)
	}
	key := engine.ResourceKey{InstallationID: instance.installation, Scope: scope, Class: engine.ResourceClassAgent, ExecutionID: "run-1", Qualifier: "opencode"}
	return instance, workspace, key
}

func launchSpec(workspace engine.Workspace, key engine.ResourceKey) engine.LaunchSpec {
	return engine.LaunchSpec{
		Key: key, Class: engine.ResourceClassAgent, Mode: engine.ExecutionModeContainer,
		Artifact: engine.Artifact{ContainerImage: "agentctl:qualified"},
		Arguments: []engine.Argument{
			{Kind: engine.ArgumentLiteral, Value: "-listen"},
			{Kind: engine.ArgumentEndpointAddress, Endpoint: "agentctl"},
		},
		Workspace:       workspace,
		Security:        engine.SecurityRequirements{ReadOnlyRoot: true, RunAsUser: 1000, RunAsGroup: 1000, DropAllCapabilities: true, NoPrivilegeEscalation: true},
		Limits:          engine.ResourceLimits{MemoryBytes: 1 << 20, NanoCPUs: 500_000_000, ProcessCount: 64},
		TemporaryMounts: []engine.TemporaryMount{{Target: "/tmp", SizeBytes: 1 << 20, Mode: 0o1777, NoSUID: true, NoDevice: true}},
		Endpoints:       []engine.Endpoint{{Name: "agentctl", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: 8081, ReadinessPath: "/health", Ingress: engine.IngressWorkerControl}},
		Requirements:    engine.Requirements{Mode: engine.ExecutionModeContainer, DurableWork: true, DurableState: true, ReadOnlyRoot: true, ReadOnlyMounts: true, TemporaryMounts: true, ProtectedWorkspaceSharing: true, ConfirmedProcessTreeStop: true},
		Sharing:         engine.WorkspaceSharing{ProtectedAncestry: true},
	}
}

func TestStartPreservesLegacyIdentityAndLifecycle(t *testing.T) {
	fake := &fakeClient{}
	instance, workspace, key := newFixture(t, fake)
	resource, err := instance.Start(context.Background(), launchSpec(workspace, key))
	if err != nil {
		t.Fatal(err)
	}
	if resource.Ref.ConcreteID != "docker-id" || resource.Endpoint != "http://127.0.0.1:49152" {
		t.Fatalf("resource = %+v", resource)
	}
	wantName := containerName("opencode", "run-1")
	if fake.created.Name != wantName || fake.created.Config.Labels["orkest.managed"] != "agentctl" || fake.created.Config.Labels["orkest.run_id_hash"] == "" {
		t.Fatalf("Docker identity = name %q labels %#v", fake.created.Name, fake.created.Config.Labels)
	}
	for _, label := range []string{"orkest.organization_id", "orkest.workspace_id", "orkest.task_id", "orkest.run_id"} {
		if _, ok := fake.created.Config.Labels[label]; ok {
			t.Fatalf("Docker identity label %q exposed a raw ID", label)
		}
	}
	if fake.created.Config.Labels["orkest.installation"] == "" {
		t.Fatal("Docker identity omitted installation label")
	}
	if len(fake.created.HostConfig.Mounts) != 3 || fake.created.HostConfig.Mounts[2].Target != "/workspace/repositories" || !fake.created.HostConfig.Mounts[2].ReadOnly {
		t.Fatalf("protected mounts = %+v", fake.created.HostConfig.Mounts)
	}
	if !fake.created.HostConfig.ReadonlyRootfs || len(fake.created.HostConfig.CapDrop) != 1 || fake.created.HostConfig.CapDrop[0] != "ALL" || len(fake.created.HostConfig.SecurityOpt) != 1 {
		t.Fatalf("Docker security policy was not applied: %+v", fake.created.HostConfig)
	}
	if fake.created.HostConfig.Memory != 1<<20 || fake.created.HostConfig.NanoCPUs != 500_000_000 || fake.created.HostConfig.PidsLimit == nil || *fake.created.HostConfig.PidsLimit != 64 || fake.created.HostConfig.Tmpfs["/tmp"] == "" {
		t.Fatalf("Docker resource limits or tmpfs were not applied: %+v", fake.created.HostConfig)
	}
	observation, err := instance.Inspect(context.Background(), engine.Lookup{Key: &key})
	if err != nil || observation.State != engine.ObservationActive || observation.Resource.Ref.ConcreteID != resource.Ref.ConcreteID {
		t.Fatalf("Inspect() = %+v, %v", observation, err)
	}
	listed, err := instance.List(context.Background(), engine.Selector{InstallationID: instance.installation, Class: engine.ResourceClassAgent, Scope: &key.Scope})
	if err != nil || len(listed) != 1 {
		t.Fatalf("List() = %+v, %v", listed, err)
	}
	if err = instance.Stop(context.Background(), resource.Ref); err != nil {
		t.Fatal(err)
	}
	observation, err = instance.Inspect(context.Background(), engine.Lookup{Key: &key})
	if err != nil || observation.State != engine.ObservationStopped {
		t.Fatalf("Inspect() after stop = %+v, %v", observation, err)
	}
	if err = instance.Cleanup(context.Background(), resource.Ref); err != nil {
		t.Fatal(err)
	}
	if !fake.removed {
		t.Fatal("stopped Docker container was not removed")
	}
}

func TestRunIDLookupFindsPreRuntimeDockerName(t *testing.T) {
	fake := &fakeClient{}
	instance, _, key := newFixture(t, fake)
	hash, _ := hashScope(key.ExecutionID)
	legacyName := "orkest-agentctl-" + hash[:24]
	fake.resource = container.InspectResponse{
		ID: "legacy-id", Name: "/" + legacyName,
		Config: &container.Config{Labels: map[string]string{
			"orkest.managed": "agentctl", "orkest.run_id_hash": hash,
			"orkest.organization_id_hash": hashValue(key.Scope.OrganizationID),
			"orkest.workspace_id_hash":    hashValue(key.Scope.WorkspaceID),
			"orkest.task_id_hash":         hashValue(key.Scope.TaskID),
		}},
		State:           &container.State{Running: true},
		NetworkSettings: &container.NetworkSettings{Ports: network.PortMap{network.MustParsePort("8081/tcp"): {{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "49152"}}}},
	}
	observation, err := instance.Inspect(context.Background(), engine.Lookup{Key: &key})
	if err != nil || observation.State != engine.ObservationActive || observation.Resource.Ref.ConcreteID != "legacy-id" {
		t.Fatalf("Inspect() legacy resource = %+v, %v", observation, err)
	}
}

func TestCreateFailureReturnsDeterministicReconciliationReference(t *testing.T) {
	fake := &fakeClient{createErr: errors.New("response lost")}
	instance, workspace, key := newFixture(t, fake)
	resource, err := instance.Start(context.Background(), launchSpec(workspace, key))
	if err == nil || resource.Ref.ConcreteID != containerName("opencode", key.ExecutionID) {
		t.Fatalf("Start() = %+v, %v", resource, err)
	}
}

func hashValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func TestResolveWorkspaceInspectionDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	fake := &fakeClient{}
	instance, err := New(Config{DataRoot: root, Client: fake})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := instance.ResolveWorkspace(context.Background(), engine.WorkspaceRequest{
		Scope: engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "missing"}, Intent: engine.WorkspaceInspect,
	})
	if err != nil || workspace.Exists {
		t.Fatalf("ResolveWorkspace() = %+v, %v", workspace, err)
	}
	if _, err = os.Stat(filepath.Join(root, "tasks")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect-only operation created task storage: %v", err)
	}
}

func TestEditorWorkspaceInspectionDoesNotRequireAgentState(t *testing.T) {
	root := t.TempDir()
	fake := &fakeClient{}
	instance, err := New(Config{DataRoot: root, Client: fake})
	if err != nil {
		t.Fatal(err)
	}
	scope := engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	work, state := taskstorage.Directories(root, scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
	if err = os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := instance.ResolveWorkspace(context.Background(), engine.WorkspaceRequest{
		Scope: scope, Intent: engine.WorkspaceInspect, Required: engine.WorkspaceGuarantees{DurableWork: true},
	})
	if err != nil || !workspace.Exists || workspace.WorkPath != work {
		t.Fatalf("Inspect existing task work without agent state = %#v, %v", workspace, err)
	}
	if _, err = os.Stat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("editor workspace inspection created agent state: %v", err)
	}
}

func TestEditorIdentityDecodesLegacyOwnershipLabels(t *testing.T) {
	fake := &fakeClient{}
	instance, _, _ := newFixture(t, fake)
	key := engine.ResourceKey{InstallationID: instance.installation, Scope: engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}, Class: engine.ResourceClassEditor, ExecutionID: strings.Repeat("a", 64), Qualifier: "task-editor"}
	metadata := engine.OwnershipMetadata{OwnerID: "user", Mode: "rw", Fingerprint: "fingerprint"}
	labels := editorLabels(instance.installation, key.Scope, key.ExecutionID, metadata)
	resource, state, err := instance.decodeEditor(container.InspectResponse{ID: "editor-container", Config: &container.Config{Labels: labels}, State: &container.State{Running: true}}, key)
	if err != nil || state != engine.ObservationActive {
		t.Fatalf("decodeEditor() = %#v, %v, %v", resource, state, err)
	}
	if resource.Ref.Key.ExecutionID != key.ExecutionID || resource.Ref.Key.Scope != key.Scope || resource.Metadata != metadata {
		t.Fatalf("decoded editor ownership = %#v / %#v", resource.Ref.Key, resource.Metadata)
	}
	wrongOwner := key
	wrongOwner.Scope.TaskID = "other-task"
	if _, _, err = instance.decodeEditor(container.InspectResponse{ID: "editor-container", Config: &container.Config{Labels: labels}, State: &container.State{Running: true}}, wrongOwner); !errors.Is(err, engine.ErrIdentityMismatch) {
		t.Fatalf("decodeEditor() with wrong scope = %v", err)
	}
}

func TestDockerEditorCapabilitiesAdvertisePrivateRelaySupport(t *testing.T) {
	instance, _, _ := newFixture(t, &fakeClient{})
	capabilities := instance.Capabilities()
	if !capabilities.GatewayOnlyIngress || !capabilities.DeniedEgress || !capabilities.PrivateHTTPWebSocketRouting {
		t.Fatalf("Docker omitted private editor relay guarantees: %#v", capabilities)
	}
}

func TestDockerEditorAdmissionRejectsUnsafeWorkerWorkspaceMounts(t *testing.T) {
	work := t.TempDir()
	repositories := filepath.Join(work, "repositories")
	repo := filepath.Join(repositories, "github", "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*container.InspectResponse)
		wantOK bool
	}{
		{name: "protected layout", wantOK: true},
		{name: "legacy layout", change: func(worker *container.InspectResponse) { delete(worker.Config.Labels, checkoutMountLayoutLabel) }},
		{name: "writable checkout parent", change: func(worker *container.InspectResponse) { worker.Mounts[1].RW = true }},
		{name: "privileged", change: func(worker *container.InspectResponse) { worker.HostConfig.Privileged = true }},
		{name: "mount capability", change: func(worker *container.InspectResponse) { worker.HostConfig.CapAdd = []string{"SYS_ADMIN"} }},
		{name: "writable alias", change: func(worker *container.InspectResponse) {
			worker.Mounts = append(worker.Mounts, container.MountPoint{Source: work, Destination: "/mnt/work", RW: true})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := container.InspectResponse{
				Config:     &container.Config{Labels: map[string]string{checkoutMountLayoutLabel: checkoutMountLayout}},
				HostConfig: &container.HostConfig{},
				Mounts: []container.MountPoint{
					{Source: work, Destination: workMountPath, RW: true},
					{Source: repositories, Destination: workMountPath + "/repositories", RW: false},
					{Source: repo, Destination: workMountPath + "/repositories/github/repo", RW: true},
				},
			}
			if tc.change != nil {
				tc.change(&worker)
			}
			err := validateProtectedWorker(worker, work)
			if (err == nil) != tc.wantOK {
				t.Fatalf("validateProtectedWorkerMounts() = %v", err)
			}
		})
	}
}

func TestDockerAgentRejectsSessionHomeMountSymlinkEscape(t *testing.T) {
	fake := &fakeClient{}
	instance, workspace, key := newFixture(t, fake)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace.StatePath, "home")); err != nil {
		t.Fatal(err)
	}
	spec := launchSpec(workspace, key)
	spec.Mounts = []engine.Mount{{SourceKind: engine.MountSourceHost, Source: "/srv/settings.xml", Target: stateMountPath + "/home/.m2/settings.xml", ReadOnly: true}}
	if _, err := instance.Start(context.Background(), spec); err == nil {
		t.Fatal("Docker agent accepted a home mount escaping through a symlink")
	}
	if fake.created.Config != nil {
		t.Fatal("Docker agent created a container before rejecting the unsafe mount")
	}
	if _, err := os.Stat(filepath.Join(outside, ".m2")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe mount modified the symlink destination: %v", err)
	}
}
