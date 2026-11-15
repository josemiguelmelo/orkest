package editor

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

type fakeEditorEngine struct {
	engine.ExecutorEngine
	workspace engine.Workspace
	spec      engine.LaunchSpec
	resource  engine.Resource
	stopped   engine.ResourceRef
}

func (f *fakeEditorEngine) Capabilities() engine.Capabilities {
	return engine.Capabilities{Modes: []engine.ExecutionMode{engine.ExecutionModeContainer}, ReadOnlyRoot: true, ReadOnlyMounts: true, TemporaryMounts: true, ProtectedWorkspaceSharing: true, GatewayOnlyIngress: true, DeniedEgress: true, PrivateHTTPWebSocketRouting: true, ConfirmedProcessTreeStop: true}
}

func (f *fakeEditorEngine) Validate(context.Context, engine.Requirements) error { return nil }

func (f *fakeEditorEngine) ResolveWorkspace(context.Context, engine.WorkspaceRequest) (engine.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeEditorEngine) Start(_ context.Context, spec engine.LaunchSpec) (engine.Resource, error) {
	f.spec = spec
	return f.resource, nil
}

func (f *fakeEditorEngine) List(context.Context, engine.Selector) ([]engine.Resource, error) {
	if f.resource.Ref.Key.ExecutionID == "" {
		return nil, nil
	}
	return []engine.Resource{f.resource}, nil
}

func (f *fakeEditorEngine) Stop(_ context.Context, ref engine.ResourceRef) error {
	f.stopped = ref
	return nil
}

func (f *fakeEditorEngine) Cleanup(context.Context, engine.ResourceRef) error { return nil }

func TestEditorExecutorBuildsProtectedGenerationLaunch(t *testing.T) {
	workPath := t.TempDir()
	canonicalWorkPath, err := filepath.EvalSymlinks(workPath)
	if err != nil {
		t.Fatal(err)
	}
	workspace := engine.Workspace{Exists: true, Identity: "task-hash", WorkPath: workPath, StatePath: "/durable/task/state"}
	fake := &fakeEditorEngine{workspace: workspace, resource: engine.Resource{
		Ref:      engine.ResourceRef{Key: engine.ResourceKey{Class: engine.ResourceClassEditor, ExecutionID: strings.Repeat("a", 64), Scope: engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}}, ConcreteID: "container-id"},
		Endpoint: "http://127.0.0.1:49152", Access: "relay-token", Metadata: engine.OwnershipMetadata{OwnerID: "user", Mode: "rw", Fingerprint: "selection"},
	}}
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
	if instance.ID != "container-id" || instance.Endpoint != "http://127.0.0.1:49152" || instance.GatewayToken != "relay-token" {
		t.Fatalf("started editor instance = %#v", instance)
	}
	spec := fake.spec
	if spec.Key.Class != engine.ResourceClassEditor || spec.Key.ExecutionID != generation || spec.Workspace.WorkPath != canonicalWorkPath || spec.Workspace.Identity != workspace.Identity {
		t.Fatalf("editor resource key/workspace = %#v / %#v", spec.Key, spec.Workspace)
	}
	if len(spec.Mounts) != 1 || spec.Mounts[0].SourceKind != engine.MountSourceWorkspace || spec.Mounts[0].Target != "/task" || spec.Mounts[0].ReadOnly {
		t.Fatalf("editor mounts = %#v", spec.Mounts)
	}
	if len(spec.GeneratedFiles) != 1 || !reflect.DeepEqual(spec.GeneratedFiles[0].Contents, manifest) || spec.GeneratedFiles[0].Mode != 0o444 {
		t.Fatalf("generated manifest = %#v", spec.GeneratedFiles)
	}
	if !spec.Security.ReadOnlyRoot || !spec.Security.DropAllCapabilities || !spec.Security.NoPrivilegeEscalation || spec.Limits.ProcessCount != 256 || spec.Endpoints[0].Ingress != engine.IngressGatewayOnly {
		t.Fatalf("editor security/limits/ingress = %#v / %#v / %#v", spec.Security, spec.Limits, spec.Endpoints)
	}
}

func TestEditorExecutorDiscoveryAndStopUseEngineIdentity(t *testing.T) {
	workspace := engine.Workspace{Exists: true, WorkPath: t.TempDir()}
	fake := &fakeEditorEngine{workspace: workspace, resource: engine.Resource{
		Ref:      engine.ResourceRef{Key: engine.ResourceKey{Class: engine.ResourceClassEditor, ExecutionID: strings.Repeat("b", 64), Scope: engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}}, ConcreteID: "container-id"},
		Metadata: engine.OwnershipMetadata{OwnerID: "user", Mode: "ro", Fingerprint: "fingerprint"},
	}}
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
	if fake.stopped.ConcreteID != "container-id" || fake.stopped.Key.ExecutionID != strings.Repeat("b", 64) {
		t.Fatalf("stopped resource ref = %#v", fake.stopped)
	}
}
