package taskeditor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

type taskReaderStub struct {
	task         taskmodel.Task
	authorizeErr error
	authorized   bool
	read         bool
}

func (s *taskReaderStub) AuthorizeExecution(context.Context, string, string, string, string, bool) error {
	s.authorized = true
	return s.authorizeErr
}

func (s *taskReaderStub) Get(context.Context, string, string, string, string) (taskmodel.Task, error) {
	s.read = true
	return s.task, nil
}

type inspectorStub struct {
	inspection Inspection
	selected   []Checkout
	called     bool
}

func (s *inspectorStub) Inspect(_ context.Context, _, _, _ string, selected []Checkout) (Inspection, error) {
	s.called = true
	s.selected = selected
	return s.inspection, nil
}

func TestDiscoverAuthorizesAndPassesOnlyTaskSelections(t *testing.T) {
	reader := &taskReaderStub{task: taskmodel.Task{Repositories: []taskmodel.RepositoryRef{{ID: "r2", ProviderName: "github", Name: "worker"}, {ID: "r1", ProviderName: "github", Name: "service"}}}}
	inspector := &inspectorStub{inspection: Inspection{Roots: []Root{{Name: "service", Path: "/workspace/repositories/github/r1", Source: "/private/source"}}, Missing: []MissingCheckout{{RegistrationID: "missing", ProviderName: "github", Name: "missing"}}}}
	got, err := Discover(context.Background(), reader, inspector, "user", "org", "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	if !reader.authorized || !reader.read || !inspector.called {
		t.Fatal("expected authorization, task read, and checkout inspection")
	}
	if len(inspector.selected) != 2 || inspector.selected[0].RegistrationID != "r1" || inspector.selected[1].RegistrationID != "r2" {
		t.Fatalf("unexpected selected checkouts: %#v", inspector.selected)
	}
	var manifest struct {
		Folders []Root `json:"folders"`
	}
	if err := json.Unmarshal(got.Manifest, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Folders) != 1 || manifest.Folders[0].Path != "/workspace/repositories/github/r1" || manifest.Folders[0].Source != "" {
		t.Fatalf("unexpected manifest: %#v", manifest.Folders)
	}
	if len(got.Missing) != 1 || got.Missing[0].RegistrationID != "missing" {
		t.Fatalf("unexpected missing set: %#v", got.Missing)
	}
}

func TestDiscoverAuthorizesBeforeTaskReadOrInspection(t *testing.T) {
	reader := &taskReaderStub{authorizeErr: errors.New("denied")}
	inspector := &inspectorStub{}
	if _, err := Discover(context.Background(), reader, inspector, "user", "org", "workspace", "task"); err == nil {
		t.Fatal("expected authorization failure")
	}
	if reader.read || inspector.called {
		t.Fatal("task read or filesystem inspection occurred before authorization")
	}
}

func TestMaterializeManifestUsesPrivateTemporaryStorage(t *testing.T) {
	path, cleanup, err := MaterializeManifest([]byte(`{"folders":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("temporary directory mode = %o", info.Mode().Perm())
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest mode = %o", info.Mode().Perm())
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("temporary manifest directory remains: %v", err)
	}
}

func TestDisplayNameDisambiguatesDuplicateRepositoryNames(t *testing.T) {
	roots := []Root{{Name: "service", Path: "/workspace/repositories/github/one"}, {Name: "service (github/two)", Path: "/workspace/repositories/github/two"}}
	if got := DisplayName(roots, "service", "github", "two"); got != "service (github/two) 2" {
		t.Fatalf("unexpected disambiguated name: %q", got)
	}
}
