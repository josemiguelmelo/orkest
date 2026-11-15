package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperationManifestIsSecretFreeAndVersioned(t *testing.T) {
	store, err := NewManifestStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan := WorkloadPlan{
		Group:     GroupIdentity{InstallationID: "install", Owner: "owner", Generation: "generation"},
		Metadata:  map[string]string{"executor": "unknown", "token": "must-not-persist"},
		Workloads: []Workload{{ID: "component", Mode: ExecutionModeContainer, Artifact: Artifact{ContainerImage: "example.invalid/image"}, Environment: map[string]string{"SECRET": "secret-value"}}},
	}
	if err = store.Record(context.Background(), plan, "starting", nil); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(store.root, "*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("manifest files = %v, err = %v", paths, err)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"must-not-persist", "secret-value", "example.invalid/image"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("manifest persisted plan value %q: %s", secret, data)
		}
	}
	var manifest OperationManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || manifest.State != "starting" || manifest.Fingerprint == "" || len(manifest.Components) != 1 || manifest.Components[0] != "component" {
		t.Fatalf("manifest = %+v", manifest)
	}
}
