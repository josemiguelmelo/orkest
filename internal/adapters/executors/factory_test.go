package executors

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	sharedagentctl "github.com/josemiguelmelo/orkest/internal/adapters/executors/agentctl"
	dockerengine "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine/docker"
	kubernetesengine "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine/kubernetes"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

type unusedKubernetesAPI struct{}

func (unusedKubernetesAPI) Do(context.Context, string, string, any) (map[string]any, int, error) {
	return nil, 404, nil
}

func TestKubernetesFactoryUsesSharedAgentExecutor(t *testing.T) {
	selected, err := New(context.Background(), Config{
		Kind: string(Kubernetes), DataRoot: t.TempDir(),
		Kubernetes: kubernetesengine.EngineConfig{
			Namespace: "orkest", InstallationID: "install", DataPVC: "task-data", UID: 1000, GID: 1000,
			Client: unusedKubernetesAPI{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = selected.Close() }()
	if _, ok := selected.AgentCtlExecutor.(*sharedagentctl.AgentCtlExecutor); !ok {
		t.Fatalf("Kubernetes agent executor = %T", selected.AgentCtlExecutor)
	}
	if selected.EditorExecutor == nil {
		t.Fatal("Kubernetes did not construct the shared editor executor")
	}
}

func TestFactorySelectsEditorCapabilitiesWithoutStartingResources(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   Kind
		editor EditorConfig
		want   bool
	}{
		{name: "Docker defaults always expose editor", kind: Docker, want: true},
		{name: "Docker editor", kind: Docker, editor: EditorConfig{Image: "editor:qualified", ReadOnlyImage: "editor:qualified-ro"}, want: true},
		{name: "Local editor policy remains available", kind: Local, editor: EditorConfig{Image: "editor:qualified", ReadOnlyImage: "editor:qualified-ro"}, want: true},
		{name: "Kubernetes editor policy remains available", kind: Kubernetes, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			config := Config{Kind: string(tc.kind), DataRoot: t.TempDir(), AgentctlBinary: binary, Editor: tc.editor}
			if tc.kind == Kubernetes {
				config.Kubernetes = kubernetesengine.EngineConfig{DataRoot: config.DataRoot, Namespace: "orkest", InstallationID: "install", DataPVC: "task-data", UID: 1000, GID: 1000, Client: unusedKubernetesAPI{}}
			}
			selected, err := New(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = selected.Close() }()
			if (selected.EditorExecutor != nil) != tc.want {
				t.Fatalf("editor capability=%v want %v", selected.EditorExecutor != nil, tc.want)
			}
			if selected.EditorExecutor != nil {
				result, err := selected.EditorExecutor.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{ProviderName: "github", RegistrationID: "repo"}})
				if err != nil || len(result.Missing) != 1 {
					t.Fatalf("selected inspector: %+v %v", result, err)
				}
			}
		})
	}
}

type ownedEngine struct {
	dockerengine.Client
	closes atomic.Int32
	err    error
}

func (e *ownedEngine) Close() error {
	e.closes.Add(1)
	return e.err
}

func TestSelectionOwnsSharedClientAcrossCopies(t *testing.T) {
	wantErr := errors.New("close failure")
	engine := &ownedEngine{err: wantErr}
	selected, err := newDockerSelection(context.Background(), Config{DataRoot: t.TempDir()}, engine)
	if err != nil {
		t.Fatal(err)
	}
	if engine.closes.Load() != 0 {
		t.Fatal("shared agent executor closed the composition-owned client")
	}
	// Concurrent cleanup of copied selections must close the owned client once,
	// preserving the result for every caller.
	var workers sync.WaitGroup
	for range 8 {
		copyOfSelection := selected
		workers.Go(func() {
			if err := copyOfSelection.Close(); !errors.Is(err, wantErr) {
				t.Errorf("close: %v", err)
			}
		})
	}
	workers.Wait()
	if engine.closes.Load() != 1 {
		t.Fatalf("client closed %d times", engine.closes.Load())
	}
}

func TestDockerSelectionCleansUpPartialConstruction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config Config
	}{
		{name: "invalid agent configuration"},
		{name: "invalid editor configuration", config: Config{Editor: EditorConfig{Image: "same", ReadOnlyImage: "same"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &ownedEngine{}
			if tc.name != "invalid agent configuration" {
				tc.config.DataRoot = t.TempDir()
			}
			if _, err := newDockerSelection(context.Background(), tc.config, engine); err == nil {
				t.Fatal("expected construction failure")
			}
			if engine.closes.Load() != 1 {
				t.Fatalf("client closed %d times", engine.closes.Load())
			}
		})
	}
}
