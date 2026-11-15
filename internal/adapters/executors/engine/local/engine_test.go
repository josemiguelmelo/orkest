package local

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
)

func TestResolveWorkspaceInspectDoesNotCreateStorage(t *testing.T) {
	root := t.TempDir()
	engineInstance, err := New(Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	request := engine.WorkspaceRequest{Scope: engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}, Intent: engine.WorkspaceInspect}
	workspace, err := engineInstance.ResolveWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Exists {
		t.Fatal("missing workspace reported as existing")
	}
	if _, err = os.Stat(filepath.Join(root, "tasks")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect-only resolution created task storage: %v", err)
	}
}

func TestNativeWorkloadIdentitySurvivesInspectionAndStops(t *testing.T) {
	root := t.TempDir()
	engineInstance, err := New(Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	scope := engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	workspace, err := engineInstance.ResolveWorkspace(context.Background(), engine.WorkspaceRequest{Scope: scope, Intent: engine.WorkspaceEnsureForAgent})
	if err != nil {
		t.Fatal(err)
	}
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep executable is not available")
	}
	spec := engine.LaunchSpec{
		Key:   engine.ResourceKey{InstallationID: engineInstance.installation, Scope: scope, Class: engine.ResourceClassAgent, ExecutionID: "run-1"},
		Class: engine.ResourceClassAgent, Mode: engine.ExecutionModeNative,
		Artifact:     engine.Artifact{ExecutablePath: sleep},
		Arguments:    []engine.Argument{{Kind: engine.ArgumentLiteral, Value: "60"}},
		Workspace:    workspace,
		Requirements: engine.Requirements{Mode: engine.ExecutionModeNative, DurableWork: true, DurableState: true},
	}
	resource, err := engineInstance.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	recreated, err := New(Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	lookup := engine.Lookup{Key: &spec.Key}
	observation, err := recreated.Inspect(context.Background(), lookup)
	if err != nil || observation.State != engine.ObservationActive || observation.Resource.Ref.ConcreteID != resource.Ref.ConcreteID {
		t.Fatalf("Inspect() = %+v, %v", observation, err)
	}
	listed, err := recreated.List(context.Background(), engine.Selector{InstallationID: engineInstance.installation, Class: engine.ResourceClassAgent})
	if err != nil || len(listed) != 1 || listed[0].Ref.ConcreteID != resource.Ref.ConcreteID {
		t.Fatalf("List() = %+v, %v", listed, err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = recreated.Stop(stopCtx, resource.Ref); err != nil {
		t.Fatal(err)
	}
	observation, err = recreated.Inspect(context.Background(), lookup)
	if err != nil || observation.State != engine.ObservationStopped {
		t.Fatalf("Inspect() after stop = %+v, %v", observation, err)
	}
}

func TestCapabilitiesDoNotClaimEditorIsolation(t *testing.T) {
	engineInstance, err := New(Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = engineInstance.Validate(context.Background(), engine.Requirements{Mode: engine.ExecutionModeNative, ReadOnlyMounts: true, DeniedEgress: true, ConfirmedProcessTreeStop: true})
	if !errors.Is(err, engine.ErrUnsupported) {
		t.Fatalf("Validate() error = %v, want unsupported isolation", err)
	}
}
