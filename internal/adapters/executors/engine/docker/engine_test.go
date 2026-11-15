package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/errdefs"
	sharededitor "github.com/josemiguelmelo/orkest/internal/adapters/executors/editor"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	client "github.com/moby/moby/client"
)

type fakeClient struct {
	Client
	created         client.ContainerCreateOptions
	resource        container.InspectResponse
	running         bool
	createErr       error
	startErr        error
	inspectErr      error
	removed         bool
	closed          int
	networks        map[string]network.Inspect
	networkOpt      client.NetworkCreateOptions
	groupContainers map[string]container.InspectResponse
	nextID          int
	hostPort        string
}

func (f *fakeClient) testHostPort() string {
	if f.hostPort != "" {
		return f.hostPort
	}
	return "49152"
}

func (f *fakeClient) ContainerCreate(_ context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	f.created = options
	if options.HostConfig != nil && options.Config.Labels["orkest.infrastructure.group-mode"] == "workloads" {
		if f.groupContainers == nil {
			f.groupContainers = make(map[string]container.InspectResponse)
		}
		f.nextID++
		id := fmt.Sprintf("group-container-%d", f.nextID)
		ports := network.PortMap{}
		for port := range options.Config.ExposedPorts {
			if options.HostConfig.PortBindings[port] != nil {
				ports[port] = []network.PortBinding{{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: f.testHostPort()}}
			}
		}
		networks := make(map[string]*network.EndpointSettings)
		if options.NetworkingConfig != nil {
			for name, endpoint := range options.NetworkingConfig.EndpointsConfig {
				networks[name] = endpoint
				if current, exists := f.networks[name]; exists {
					current.Containers[id] = network.EndpointResource{}
					f.networks[name] = current
				}
			}
		}
		resource := container.InspectResponse{ID: id, Name: "/" + options.Name, Config: options.Config, HostConfig: options.HostConfig, State: &container.State{}, NetworkSettings: &container.NetworkSettings{Ports: ports, Networks: networks}}
		f.groupContainers[id] = resource
		return client.ContainerCreateResult{ID: id}, nil
	}
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

func TestSharedEditorRunsAndReconcilesOnGenericDockerPlan(t *testing.T) {
	var receivedToken string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		receivedToken = request.Header.Get("X-Orkest-Editor-Relay")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	_, port, ok := strings.Cut(strings.TrimPrefix(server.URL, "http://"), ":")
	if !ok {
		t.Fatalf("parse readiness endpoint %q", server.URL)
	}
	fake := &fakeClient{hostPort: port}
	instance, err := New(Config{DataRoot: t.TempDir(), ContainerUser: "1000:1000", Client: fake})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = instance.Close() }()
	scope := taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	storageID := taskstorage.Identity(scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
	bindings, err := instance.ResolveStorage(context.Background(), engine.StorageRequest{Owner: storageID, Generation: "editor-inspection", Bindings: []engine.StorageBindingRequest{{Name: "work", StorageID: storageID, Subpath: "work", Create: true, Durable: true}}})
	if err != nil {
		t.Fatal(err)
	}
	canonicalWorkPath, err := filepath.EvalSymlinks(bindings[0].HostPath)
	if err != nil {
		t.Fatal(err)
	}
	editorExecutor, err := sharededitor.New(instance, sharededitor.Config{Image: "editor:rw", ReadOnlyImage: "editor:ro", RunAsUser: 1000, RunAsGroup: 1000})
	if err != nil {
		t.Fatal(err)
	}
	generation := strings.Repeat("d", 64)
	started, err := editorExecutor.StartEditor(context.Background(), taskeditor.Spec{
		Instance:  taskeditor.Instance{Scope: scope, UserID: "user", Generation: generation, Fingerprint: "fingerprint", Mode: taskeditor.ReadWrite},
		Discovery: taskeditor.Discovery{Roots: []taskeditor.Root{{Path: "/task", StorageRef: canonicalWorkPath}}, Manifest: []byte(`{"folders":[{"path":"/task"}]}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.GatewayToken == "" || receivedToken != started.GatewayToken || !started.GatewayTokenRequired {
		t.Fatalf("editor gateway credential = %q, readiness saw %q, required=%v", started.GatewayToken, receivedToken, started.GatewayTokenRequired)
	}
	observed, found, err := editorExecutor.InspectEditor(context.Background(), scope)
	if err != nil || !found || observed.ID != started.ID || observed.GatewayToken != started.GatewayToken {
		t.Fatalf("reconciled editor = %#v, found=%v, err=%v", observed, found, err)
	}
	if len(fake.groupContainers) != 2 || len(fake.networks) != 1 || !fake.networkOpt.Internal {
		t.Fatalf("generic editor resources = containers:%d networks:%d network:%+v", len(fake.groupContainers), len(fake.networks), fake.networkOpt)
	}
	if err = editorExecutor.StopEditor(context.Background(), started); err != nil {
		t.Fatal(err)
	}
	if len(fake.groupContainers) != 0 || len(fake.networks) != 0 {
		t.Fatalf("generic editor cleanup left resources: containers:%d networks:%d", len(fake.groupContainers), len(fake.networks))
	}
}

func newEngineFixture(t *testing.T, fake *fakeClient) *Engine {
	t.Helper()
	instance, err := New(Config{DataRoot: t.TempDir(), ContainerUser: "1000:1000", Client: fake})
	if err != nil {
		t.Fatal(err)
	}
	return instance
}

func (f *fakeClient) ContainerStart(_ context.Context, id string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
	if resource, ok := f.groupContainers[id]; ok {
		resource.State.Running = true
		f.groupContainers[id] = resource
		return client.ContainerStartResult{}, nil
	}
	for resourceID, resource := range f.groupContainers {
		if strings.TrimPrefix(resource.Name, "/") == id {
			resource.State.Running = true
			f.groupContainers[resourceID] = resource
			return client.ContainerStartResult{}, nil
		}
	}
	f.running = true
	if f.resource.State != nil {
		f.resource.State.Running = true
	}
	return client.ContainerStartResult{}, f.startErr
}

func (f *fakeClient) ContainerStop(_ context.Context, id string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	if resource, ok := f.groupContainers[id]; ok {
		resource.State.Running = false
		f.groupContainers[id] = resource
		return client.ContainerStopResult{}, nil
	}
	for resourceID, resource := range f.groupContainers {
		if strings.TrimPrefix(resource.Name, "/") == id {
			resource.State.Running = false
			f.groupContainers[resourceID] = resource
			return client.ContainerStopResult{}, nil
		}
	}
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
		if resource, ok := f.groupContainers[id]; ok {
			return client.ContainerInspectResult{Container: resource}, nil
		}
		return client.ContainerInspectResult{}, errdefs.ErrNotFound
	}
	if resource, ok := f.groupContainers[id]; ok {
		return client.ContainerInspectResult{Container: resource}, nil
	}
	if id != f.resource.ID && strings.TrimPrefix(f.resource.Name, "/") != id {
		return client.ContainerInspectResult{}, errdefs.ErrNotFound
	}
	return client.ContainerInspectResult{Container: f.resource}, nil
}

func (f *fakeClient) ContainerRemove(_ context.Context, id string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	if _, ok := f.groupContainers[id]; ok {
		delete(f.groupContainers, id)
		return client.ContainerRemoveResult{}, nil
	}
	f.removed = true
	return client.ContainerRemoveResult{}, nil
}

func (f *fakeClient) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	items := make([]container.Summary, 0, len(f.groupContainers)+1)
	for id, resource := range f.groupContainers {
		items = append(items, container.Summary{ID: id, Labels: resource.Config.Labels})
	}
	if !f.removed && f.resource.ID != "" {
		items = append(items, container.Summary{ID: f.resource.ID, Labels: f.resource.Config.Labels})
	}
	return client.ContainerListResult{Items: items}, nil
}

func (f *fakeClient) NetworkCreate(_ context.Context, name string, options client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
	if f.networks == nil {
		f.networks = make(map[string]network.Inspect)
	}
	f.networkOpt = options
	resource := network.Inspect{Network: network.Network{ID: "network-id", Name: name, Driver: options.Driver, Internal: options.Internal, Labels: options.Labels}, Containers: make(map[string]network.EndpointResource)}
	f.networks[name] = resource
	return client.NetworkCreateResult{ID: resource.ID}, nil
}

func (f *fakeClient) NetworkInspect(_ context.Context, id string, _ client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
	for name, resource := range f.networks {
		if id == name || id == resource.ID {
			return client.NetworkInspectResult{Network: resource}, nil
		}
	}
	return client.NetworkInspectResult{}, errdefs.ErrNotFound
}

func (f *fakeClient) NetworkConnect(_ context.Context, id string, options client.NetworkConnectOptions) (client.NetworkConnectResult, error) {
	for name, resource := range f.networks {
		if id == name || id == resource.ID {
			resource.Containers[options.Container] = network.EndpointResource{}
			f.networks[name] = resource
			return client.NetworkConnectResult{}, nil
		}
	}
	return client.NetworkConnectResult{}, errdefs.ErrNotFound
}

func (f *fakeClient) NetworkDisconnect(_ context.Context, id string, options client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error) {
	for name, resource := range f.networks {
		if id == name || id == resource.ID {
			delete(resource.Containers, options.Container)
			f.networks[name] = resource
			return client.NetworkDisconnectResult{}, nil
		}
	}
	return client.NetworkDisconnectResult{}, errdefs.ErrNotFound
}

func (f *fakeClient) NetworkRemove(_ context.Context, id string, _ client.NetworkRemoveOptions) (client.NetworkRemoveResult, error) {
	for name, resource := range f.networks {
		if id == name || id == resource.ID {
			delete(f.networks, name)
			return client.NetworkRemoveResult{}, nil
		}
	}
	return client.NetworkRemoveResult{}, errdefs.ErrNotFound
}

func (f *fakeClient) NetworkList(context.Context, client.NetworkListOptions) (client.NetworkListResult, error) {
	items := make([]network.Summary, 0, len(f.networks))
	for _, resource := range f.networks {
		items = append(items, network.Summary{Network: resource.Network})
	}
	return client.NetworkListResult{Items: items}, nil
}

func (*fakeClient) ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	return io.NopCloser(&emptyReader{}), nil
}

func (f *fakeClient) Close() error { f.closed++; return nil }

type emptyReader struct{}

func (*emptyReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestGenericStorageAccessAdmissionInspectsManagedWorkloads(t *testing.T) {
	fake := &fakeClient{}
	instance := newEngineFixture(t, fake)
	storageID := taskstorage.Identity("org", "workspace", "task")
	work, _ := taskstorage.Paths(instance.config.DataRoot, storageID)
	repositories := filepath.Join(work, "repositories")
	repo := filepath.Join(repositories, "github", "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	worker := container.InspectResponse{
		ID:         "managed-worker",
		Config:     &container.Config{Labels: map[string]string{groupInstallLabel: instance.installation, groupLabel: "worker-group", groupOwnerLabel: "owner", groupGenerationLabel: "generation", "orkest.infrastructure.component": "worker"}},
		HostConfig: &container.HostConfig{},
		Mounts: []container.MountPoint{
			{Source: work, Destination: "/workspace", RW: false},
			{Source: repositories, Destination: "/workspace/repositories", RW: false},
			{Source: repo, Destination: "/workspace/repositories/github/repo", RW: true},
		},
	}
	fake.groupContainers = map[string]container.InspectResponse{worker.ID: worker}
	if err := instance.ValidateStorageAccess(context.Background(), engine.StorageAccessRequest{StorageID: storageID, Subpath: "work"}); err != nil {
		t.Fatalf("protected generic worker storage layout was rejected: %v", err)
	}
	worker.HostConfig.Privileged = true
	fake.groupContainers[worker.ID] = worker
	if err := instance.ValidateStorageAccess(context.Background(), engine.StorageAccessRequest{StorageID: storageID, Subpath: "work"}); err == nil {
		t.Fatal("storage admission accepted a privileged managed workload")
	}
}

func TestInfrastructureEngineLifecycleDoesNotDependOnExecutorMetadata(t *testing.T) {
	fake := &fakeClient{}
	instance := newEngineFixture(t, fake)
	defer func() {
		if closeErr := instance.Close(); closeErr != nil {
			t.Errorf("close Docker engine: %v", closeErr)
		}
	}()
	secretPath := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(secretPath, []byte("never-copy-secret-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	hostMountSource := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(hostMountSource, []byte("external config"), 0o600); err != nil {
		t.Fatal(err)
	}
	instance.config.CredentialSources = map[string]engine.CredentialSource{"opaque-token": {FilePath: secretPath}}
	plan := engine.WorkloadPlan{
		Group:    engine.GroupIdentity{InstallationID: instance.installation, Owner: "opaque-owner", Generation: "one"},
		Metadata: map[string]string{"executor": "unrecognized-future-executor"},
		Storage:  []engine.StorageAttachment{{WorkloadID: "worker", Binding: "work", Target: "/workspace", Create: true, Durable: true}},
		Files:    []engine.GeneratedFile{{Name: "generated-config", Target: "/etc/generated.conf", Contents: []byte("generated config"), Mode: 0o444}},
		Workloads: []engine.Workload{{
			ID: "worker", Mode: engine.ExecutionModeContainer,
			Artifact:    engine.Artifact{ContainerImage: "example.invalid/arbitrary:1"},
			Endpoints:   []engine.Endpoint{{Name: "control", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: 8081, Exposure: engine.ExposurePlatform}},
			Credentials: []engine.CredentialBinding{{Name: "opaque-token", Target: "/run/credential/token", EnvironmentName: "TOKEN_FILE", ReadOnly: true}},
			Mounts:      []engine.Mount{{SourceKind: engine.MountSourceHost, Source: hostMountSource, Target: "/etc/external.conf", ReadOnly: true}},
			Arguments:   []engine.Argument{{Kind: engine.ArgumentEndpointAddress, Endpoint: "control"}},
		}},
	}
	ctx := context.Background()
	group, err := instance.StartPlan(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.created.Config.Image; got != "example.invalid/arbitrary:1" {
		t.Fatalf("created image = %q", got)
	}
	if got := fake.created.Config.Cmd; len(got) != 1 || got[0] != "0.0.0.0:8081" {
		t.Fatalf("created command = %#v", got)
	}
	credentialMountFound := false
	storageMountFound := false
	hostMountFound := false
	generatedMountFound := false
	for _, item := range fake.created.HostConfig.Mounts {
		credentialMountFound = credentialMountFound || item.Source == secretPath && item.Target == "/run/credential/token"
		storageMountFound = storageMountFound || item.Target == "/workspace" && item.ReadOnly == false
		hostMountFound = hostMountFound || item.Source == hostMountSource && item.Target == "/etc/external.conf"
		generatedMountFound = generatedMountFound || item.Target == "/etc/generated.conf"
	}
	if len(fake.created.HostConfig.Mounts) != 4 || !credentialMountFound || !storageMountFound || !hostMountFound || !generatedMountFound {
		t.Fatalf("generic mounts = %#v", fake.created.HostConfig.Mounts)
	}
	if strings.Contains(strings.Join(fake.created.Config.Env, "\n"), "never-copy-secret-value") {
		t.Fatal("credential contents were copied to the Docker environment")
	}
	observation, err := instance.InspectGroup(ctx, group.Ref)
	if err != nil || observation.State != engine.GroupActive {
		t.Fatalf("inspect group = %#v, %v", observation, err)
	}
	if observation.Group.Endpoints["control"] != "http://127.0.0.1:49152" {
		t.Fatalf("endpoint map = %#v", observation.Group.Endpoints)
	}
	if err = instance.StopGroup(ctx, group.Ref); err != nil {
		t.Fatal(err)
	}
	if err = instance.CleanupGroup(ctx, group.Ref); err != nil {
		t.Fatal(err)
	}
	generatedPath := ""
	for _, item := range fake.created.HostConfig.Mounts {
		if item.Target == "/etc/generated.conf" {
			generatedPath = item.Source
		}
	}
	if generatedPath == "" {
		t.Fatal("generated file source mount was not recorded")
	}
	if _, err = os.Stat(generatedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("generated file remains after cleanup: %v", err)
	}
}

func TestInfrastructureEngineOwnsAndCleansDeniedEgressNetwork(t *testing.T) {
	fake := &fakeClient{}
	instance := newEngineFixture(t, fake)
	defer func() { _ = instance.Close() }()
	plan := engine.WorkloadPlan{
		Group:     engine.GroupIdentity{InstallationID: instance.installation, Owner: "future-executor", Generation: "networked-workload"},
		Networks:  []engine.NetworkPlan{{ID: "private", Members: []string{"web"}, DenyEgress: true}},
		Workloads: []engine.Workload{{ID: "web", Mode: engine.ExecutionModeContainer, Artifact: engine.Artifact{ContainerImage: "web:1"}}},
	}
	group, err := instance.StartPlan(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if fake.networkOpt.Driver != "bridge" || !fake.networkOpt.Internal {
		t.Fatalf("private network options = %+v", fake.networkOpt)
	}
	if fake.created.HostConfig.NetworkMode != container.NetworkMode(infrastructureNetworkName(plan.Group)) {
		t.Fatalf("workload network mode = %q", fake.created.HostConfig.NetworkMode)
	}
	listed, err := instance.ListGroups(context.Background(), engine.Selector{InstallationID: instance.installation})
	if err != nil || len(listed) != 1 || listed[0].Ref.Identity != plan.Group {
		t.Fatalf("list partially represented generic group = %#v, %v", listed, err)
	}
	if err = instance.StopGroup(context.Background(), group.Ref); err != nil {
		t.Fatal(err)
	}
	if err = instance.CleanupGroup(context.Background(), group.Ref); err != nil {
		t.Fatal(err)
	}
	if len(fake.networks) != 0 {
		t.Fatalf("owned Docker network remains after cleanup: %+v", fake.networks)
	}
}

func TestInfrastructureEngineRunsGenericMultiWorkloadNetworkGroup(t *testing.T) {
	fake := &fakeClient{}
	instance := newEngineFixture(t, fake)
	defer func() { _ = instance.Close() }()
	plan := engine.WorkloadPlan{
		Group:    engine.GroupIdentity{InstallationID: instance.installation, Owner: "future-executor", Generation: "relay-group"},
		Networks: []engine.NetworkPlan{{ID: "private", Members: []string{"web", "router"}, DenyEgress: true}},
		Workloads: []engine.Workload{
			{ID: "web", Mode: engine.ExecutionModeContainer, Artifact: engine.Artifact{ContainerImage: "web:1"}, Endpoints: []engine.Endpoint{{Name: "http", Protocol: "http", ListenPort: 8080}}},
			{ID: "router", Mode: engine.ExecutionModeContainer, Artifact: engine.Artifact{ContainerImage: "router:1", ContainerEntrypoint: []string{"node", "relay.cjs"}}, Environment: map[string]string{"ACCESS_KEY": "ephemeral"}, Arguments: []engine.Argument{{Kind: engine.ArgumentEndpointAddress, Workload: "web", Endpoint: "http"}}, Endpoints: []engine.Endpoint{{Name: "gateway", Protocol: "http", ListenPort: 8081, Exposure: engine.ExposurePlatform}}},
		},
	}
	group, err := instance.StartPlan(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Components) != 2 || group.Endpoints["web.http"] != "http://"+infrastructureWorkloadName(plan.Group, "web")+":8080" || group.Endpoints["gateway"] != "http://127.0.0.1:49152" {
		t.Fatalf("multi-workload group = %#v", group)
	}
	var router container.InspectResponse
	for _, resource := range fake.groupContainers {
		if resource.Config.Labels["orkest.infrastructure.component"] == "router" {
			router = resource
		}
	}
	if len(router.Config.Cmd) != 1 || router.Config.Cmd[0] != "http://"+infrastructureWorkloadName(plan.Group, "web")+":8080" || len(router.HostConfig.PortBindings) != 1 {
		t.Fatalf("generic forwarding workload config = %#v / %#v", router.Config, router.HostConfig)
	}
	observed, err := instance.InspectGroup(context.Background(), group.Ref)
	if err != nil || observed.State != engine.GroupActive || len(observed.Group.Components) != 2 {
		t.Fatalf("inspect workload group = %#v, %v", observed, err)
	}
	if err = instance.StopGroup(context.Background(), group.Ref); err != nil {
		t.Fatal(err)
	}
	if err = instance.CleanupGroup(context.Background(), group.Ref); err != nil {
		t.Fatal(err)
	}
	if len(fake.groupContainers) != 0 || len(fake.networks) != 0 {
		t.Fatalf("group cleanup left resources: containers=%d networks=%d", len(fake.groupContainers), len(fake.networks))
	}
}
