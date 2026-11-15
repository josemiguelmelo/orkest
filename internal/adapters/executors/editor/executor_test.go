package editor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

type fakeEditorInfrastructure struct {
	engine.InfrastructureEngine
	storagePath   string
	plan          engine.WorkloadPlan
	group         engine.ResourceGroup
	storageAccess engine.StorageAccessRequest
	endpoint      string
	stopped       bool
	cleaned       bool
}

func (f *fakeEditorInfrastructure) Capabilities() engine.Capabilities {
	return engine.Capabilities{Modes: []engine.ExecutionMode{engine.ExecutionModeContainer}, DurableWork: true, ReadOnlyRoot: true, ReadOnlyMounts: true, TemporaryMounts: true, ProtectedStorageSharing: true, PlatformIngress: true, DeniedEgress: true, PrivateServiceRouting: true, ConfirmedProcessTreeStop: true}
}

func (f *fakeEditorInfrastructure) ResolveStorage(_ context.Context, request engine.StorageRequest) ([]engine.StorageBinding, error) {
	result := make([]engine.StorageBinding, 0, len(request.Bindings))
	for _, binding := range request.Bindings {
		result = append(result, engine.StorageBinding{Name: binding.Name, Identity: binding.StorageID, Subpath: binding.Subpath, HostPath: f.storagePath})
	}
	return result, nil
}

func (*fakeEditorInfrastructure) Close() error { return nil }

func (f *fakeEditorInfrastructure) ValidatePlan(_ context.Context, plan engine.WorkloadPlan) error {
	f.plan = plan
	return engine.ValidateWorkloadPlan(plan)
}

func (f *fakeEditorInfrastructure) ValidateStorageAccess(_ context.Context, request engine.StorageAccessRequest) error {
	f.storageAccess = request
	return nil
}

func (f *fakeEditorInfrastructure) StartPlan(_ context.Context, plan engine.WorkloadPlan) (engine.ResourceGroup, error) {
	f.group = engine.ResourceGroup{
		Ref:       engine.GroupRef{Identity: engine.GroupIdentity{InstallationID: "installation", Owner: plan.Group.Owner, Generation: plan.Group.Generation}, Provider: "provider-group"},
		Endpoints: map[string]string{"gateway": f.endpoint}, Metadata: plan.Metadata,
	}
	return f.group, nil
}

func (f *fakeEditorInfrastructure) InspectGroup(_ context.Context, ref engine.GroupRef) (engine.GroupObservation, error) {
	if f.group.Ref.Identity.Owner != ref.Identity.Owner || f.group.Ref.Identity.Generation != ref.Identity.Generation {
		return engine.GroupObservation{State: engine.GroupAbsent}, nil
	}
	return engine.GroupObservation{State: engine.GroupActive, Group: f.group}, nil
}

func (f *fakeEditorInfrastructure) ListGroups(_ context.Context, selector engine.Selector) ([]engine.ResourceGroup, error) {
	if f.group.Ref.Identity.Owner == "" || selector.GroupOwner != "" && f.group.Ref.Identity.Owner != selector.GroupOwner {
		return nil, nil
	}
	return []engine.ResourceGroup{f.group}, nil
}

func (f *fakeEditorInfrastructure) StopGroup(context.Context, engine.GroupRef) error {
	f.stopped = true
	return nil
}

func (f *fakeEditorInfrastructure) CleanupGroup(context.Context, engine.GroupRef) error {
	f.cleaned = true
	return nil
}

func TestEditorExecutorBuildsProtectedGenerationLaunch(t *testing.T) {
	workPath := t.TempDir()
	canonicalWorkPath, err := filepath.EvalSymlinks(workPath)
	if err != nil {
		t.Fatal(err)
	}
	var observedToken string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observedToken = request.Header.Get("X-Orkest-Editor-Relay")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	fake := &fakeEditorInfrastructure{storagePath: workPath, endpoint: server.URL}
	executor, err := New(fake, Config{Image: "editor:rw", ReadOnlyImage: "editor:ro", RunAsUser: 1000, RunAsGroup: 1000})
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"folders":[{"path":"/task"}]}`)
	generation := strings.Repeat("a", 64)
	instance, err := executor.StartEditor(context.Background(), taskeditor.Spec{Instance: taskeditor.Instance{Scope: taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}, UserID: "user", Generation: generation, Fingerprint: "selection", Mode: taskeditor.ReadWrite}, Discovery: taskeditor.Discovery{Roots: []taskeditor.Root{{Path: "/task", StorageRef: canonicalWorkPath}}, Manifest: manifest}})
	if err != nil {
		t.Fatal(err)
	}
	if _, decodeErr := engine.DecodeGroupRef(instance.ID); decodeErr != nil || instance.Endpoint != server.URL || instance.GatewayToken == "" || observedToken != instance.GatewayToken {
		t.Fatalf("started editor instance = %#v", instance)
	}
	plan := fake.plan
	if plan.Group.Generation != generation || len(plan.Workloads) != 2 || len(plan.Files) != 1 || !reflect.DeepEqual(plan.Files[0].Contents, manifest) || plan.Files[0].Mode != 0o444 {
		t.Fatalf("editor workload plan = %#v", plan)
	}
	editor := plan.Workloads[0]
	if len(plan.Storage) != 1 || plan.Storage[0].Target != "/task" || plan.Storage[0].ReadOnly {
		t.Fatalf("editor storage = %#v", plan.Storage)
	}
	if !editor.Security.ReadOnlyRoot || !editor.Security.DropAllCapabilities || !editor.Security.NoPrivilegeEscalation || editor.Limits.ProcessCount != 256 || plan.Workloads[1].Endpoints[0].Exposure != engine.ExposurePlatform {
		t.Fatalf("editor security/limits/ingress = %#v / %#v / %#v", editor.Security, editor.Limits, plan.Workloads[1].Endpoints)
	}
}

func TestEditorExecutorDiscoveryAndStopUseEngineIdentity(t *testing.T) {
	fake := &fakeEditorInfrastructure{storagePath: t.TempDir()}
	owner := taskstorage.Identity("org", "workspace", "task")
	fake.group = engine.ResourceGroup{Ref: engine.GroupRef{Identity: engine.GroupIdentity{InstallationID: "installation", Owner: owner, Generation: strings.Repeat("b", 64)}, Provider: "provider-group"}, Metadata: map[string]string{"executor": "task-editor", "organization": "org", "workspace": "workspace", "task": "task", "user": "user", "mode": "ro", "fingerprint": "fingerprint"}, Endpoints: map[string]string{"gateway": "http://127.0.0.1:49152"}}
	executor, err := New(fake, Config{Image: "editor:rw", ReadOnlyImage: "editor:ro", RunAsUser: 1000, RunAsGroup: 1000})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := executor.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{ProviderName: "github", RegistrationID: "repo"}})
	if err != nil || len(inspection.Roots) != 1 {
		t.Fatalf("Inspect() = %#v, %v", inspection, err)
	}
	instance, found, err := executor.InspectEditor(context.Background(), taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"})
	if err != nil || !found || instance.Generation != strings.Repeat("b", 64) || instance.UserID != "user" {
		t.Fatalf("InspectEditor() = %#v, %v, %v", instance, found, err)
	}
	if err = executor.StopEditor(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if !fake.stopped || !fake.cleaned {
		t.Fatalf("generic editor group stop/cleanup = %v/%v", fake.stopped, fake.cleaned)
	}
}

func TestEditorExecutorPlansGenericPrivateWorkloadGroup(t *testing.T) {
	workPath := t.TempDir()
	canonicalWorkPath, err := filepath.EvalSymlinks(workPath)
	if err != nil {
		t.Fatal(err)
	}
	var observedToken string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observedToken = request.Header.Get("X-Orkest-Editor-Relay")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	infrastructure := &fakeEditorInfrastructure{endpoint: server.URL}
	infrastructure.storagePath = workPath
	executor, err := New(infrastructure, Config{Image: "editor:rw", ReadOnlyImage: "editor:ro", RunAsUser: 1000, RunAsGroup: 1000})
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"folders":[{"path":"/task"}]}`)
	generation := strings.Repeat("c", 64)
	instance, err := executor.StartEditor(context.Background(), taskeditor.Spec{
		Instance:  taskeditor.Instance{Scope: taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}, UserID: "user", Generation: generation, Fingerprint: "fingerprint", Mode: taskeditor.ReadWrite},
		Discovery: taskeditor.Discovery{Roots: []taskeditor.Root{{Path: "/task", StorageRef: canonicalWorkPath}}, Manifest: manifest},
	})
	if err != nil {
		t.Fatal(err)
	}
	if instance.ID == "" || instance.Endpoint != server.URL || !instance.GatewayTokenRequired || instance.GatewayToken == "" || observedToken != instance.GatewayToken {
		t.Fatalf("generic editor instance/token = %#v, observed %q", instance, observedToken)
	}
	if len(infrastructure.plan.Workloads) != 2 || len(infrastructure.plan.Networks) != 1 || !infrastructure.plan.Networks[0].DenyEgress {
		t.Fatalf("generic editor plan workloads/network = %#v / %#v", infrastructure.plan.Workloads, infrastructure.plan.Networks)
	}
	if !strings.Contains(infrastructure.plan.Workloads[1].Environment["ORKEST_EDITOR_RELAY_TOKEN"], instance.GatewayToken) || infrastructure.plan.Workloads[1].EnvironmentEndpointRefs["ORKEST_EDITOR_RELAY_UPSTREAM"].Workload != "editor" {
		t.Fatalf("generic forwarding workload plan = %#v", infrastructure.plan.Workloads[1])
	}
	encodedMetadata, _ := json.Marshal(infrastructure.plan.Metadata)
	if strings.Contains(string(encodedMetadata), instance.GatewayToken) {
		t.Fatal("editor gateway token was copied to generic metadata")
	}
	if !infrastructure.plan.Storage[0].Durable || infrastructure.plan.Storage[0].ReadOnly {
		t.Fatalf("editor storage attachment = %#v", infrastructure.plan.Storage)
	}
	access := taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	found, ok, err := executor.InspectEditor(context.Background(), access)
	if err != nil || !ok || found.GatewayToken != instance.GatewayToken || found.ID != instance.ID {
		t.Fatalf("generic editor inspection = %#v, %v, %v", found, ok, err)
	}
	if err = executor.StopEditor(context.Background(), instance); err != nil || !infrastructure.stopped || !infrastructure.cleaned {
		t.Fatalf("generic editor stop/cleanup = %v, stopped=%v cleaned=%v", err, infrastructure.stopped, infrastructure.cleaned)
	}
}
