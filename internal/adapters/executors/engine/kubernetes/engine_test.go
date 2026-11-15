package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
)

type fakeAPI struct {
	resources      map[string]map[string]any
	failCreateKind string
}

func newFakeAPI() *fakeAPI { return &fakeAPI{resources: make(map[string]map[string]any)} }

func (f *fakeAPI) Do(_ context.Context, method, requestPath string, body any) (map[string]any, int, error) {
	if method == http.MethodGet && (strings.HasSuffix(requestPath, "/pods") || strings.HasSuffix(requestPath, "/services") || strings.HasSuffix(requestPath, "/configmaps")) {
		items := make([]any, 0)
		for path, resource := range f.resources {
			if strings.HasPrefix(path, requestPath+"/") {
				items = append(items, resource)
			}
		}
		return map[string]any{"items": items}, http.StatusOK, nil
	}
	if method == http.MethodGet {
		resource, ok := f.resources[requestPath]
		if !ok {
			return nil, http.StatusNotFound, nil
		}
		return cloneResource(resource), http.StatusOK, nil
	}
	if method == http.MethodPost {
		resource := cloneResource(body.(map[string]any))
		if resource["kind"] == f.failCreateKind {
			return nil, http.StatusInternalServerError, errors.New("injected Kubernetes create failure")
		}
		metadata, _ := resource["metadata"].(map[string]any)
		name, _ := metadata["name"].(string)
		path := requestPath + "/" + name
		metadata["uid"] = "pod-uid"
		if resource["kind"] == "Pod" {
			resource["status"] = map[string]any{"phase": "Running", "containerStatuses": []any{map[string]any{"ready": true}}}
		}
		f.resources[path] = resource
		return cloneResource(resource), http.StatusCreated, nil
	}
	if method == http.MethodDelete {
		delete(f.resources, requestPath)
		return nil, http.StatusOK, nil
	}
	return nil, http.StatusMethodNotAllowed, nil
}

func TestKubernetesInfrastructureEngineRunsAnUnrecognizedWorkload(t *testing.T) {
	api := newFakeAPI()
	instance, err := New(EngineConfig{DataRoot: t.TempDir(), Namespace: "orkest", InstallationID: "install", DataPVC: "task-data", UID: 1000, GID: 1000, CredentialSources: map[string]engine.CredentialSource{"opaque-token": {Secret: "credential-secret", Key: "auth"}}, Client: api})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := instance.Close(); closeErr != nil {
			t.Errorf("close Kubernetes engine: %v", closeErr)
		}
	}()
	plan := engine.WorkloadPlan{
		Group:    engine.GroupIdentity{InstallationID: "install", Owner: "opaque-owner", Generation: "generation-1"},
		Metadata: map[string]string{"executor": "future-executor"},
		Storage:  []engine.StorageAttachment{{WorkloadID: "arbitrary-worker", Binding: "work", Target: "/workspace", Create: true, Durable: true}},
		Files:    []engine.GeneratedFile{{Name: "workspace-manifest", Target: "/home/editor/workspace.code-workspace", Contents: []byte("{}"), Mode: 0o444}},
		Workloads: []engine.Workload{{
			ID: "arbitrary-worker", Mode: engine.ExecutionModeContainer,
			Artifact:    engine.Artifact{ContainerImage: "example.invalid/arbitrary:1"},
			Endpoints:   []engine.Endpoint{{Name: "control", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: 9001, ReadinessPath: "/ready", Exposure: engine.ExposurePrivate}},
			Credentials: []engine.CredentialBinding{{Name: "opaque-token", Target: "/run/credential/token", EnvironmentName: "TOKEN_FILE", ReadOnly: true}},
			Arguments:   []engine.Argument{{Kind: engine.ArgumentEndpointAddress, Endpoint: "control"}},
		}},
	}
	ctx := context.Background()
	group, err := instance.StartPlan(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	name, _, err := instance.parseID(group.Ref.Provider)
	if err != nil {
		t.Fatal(err)
	}
	created := api.resources[instance.resourcePath("pods", name)]
	containerSpec := nested(created, "spec", "containers").([]any)[0].(map[string]any)
	if containerSpec["image"] != "example.invalid/arbitrary:1" {
		t.Fatalf("created container = %#v", containerSpec)
	}
	mounts := containerSpec["volumeMounts"].([]any)
	credentialMountFound := false
	for _, raw := range mounts {
		mount, _ := raw.(map[string]any)
		credentialMountFound = credentialMountFound || mount["mountPath"] == "/run/credential/token"
	}
	if len(mounts) != 3 || !credentialMountFound {
		t.Fatalf("credential mounts = %#v", mounts)
	}
	volumes := nested(created, "spec", "volumes").([]any)
	credentialVolumeFound := false
	storageVolumeFound := false
	for _, raw := range volumes {
		volume, _ := raw.(map[string]any)
		credentialVolumeFound = credentialVolumeFound || nested(volume, "secret", "secretName") == "credential-secret"
		storageVolumeFound = storageVolumeFound || nested(volume, "persistentVolumeClaim", "claimName") == "task-data"
	}
	if len(volumes) != 3 || !credentialVolumeFound {
		t.Fatalf("credential volumes = %#v", volumes)
	}
	if !storageVolumeFound {
		t.Fatalf("storage volume = %#v", volumes)
	}
	fileName := name + "-files"
	if api.resources[instance.resourcePath("configmaps", fileName)]["data"].(map[string]any)["file-"+kubernetesStorageIdentity("workspace-manifest", "generated", "key")[:12]] != "{}" {
		t.Fatal("generated file ConfigMap did not preserve the plan contents")
	}
	if group.Endpoints["control"] != "http://"+name+".orkest.svc:9001" {
		t.Fatalf("endpoint map = %#v", group.Endpoints)
	}
	observation, err := instance.InspectGroup(ctx, group.Ref)
	if err != nil || observation.State != engine.GroupActive {
		t.Fatalf("inspect group = %#v, %v", observation, err)
	}
	listed, err := instance.ListGroups(ctx, engine.Selector{InstallationID: "install", GroupOwner: "opaque-owner"})
	if err != nil || len(listed) != 1 {
		t.Fatalf("list groups = %#v, %v", listed, err)
	}
	if err = instance.StopGroup(ctx, group.Ref); err != nil {
		t.Fatal(err)
	}
	listed, err = instance.ListGroups(ctx, engine.Selector{InstallationID: "install", GroupOwner: "opaque-owner"})
	if err != nil || len(listed) != 1 {
		t.Fatalf("list stopped groups = %#v, %v", listed, err)
	}
	if err = instance.CleanupGroup(ctx, group.Ref); err != nil {
		t.Fatal(err)
	}
	listed, err = instance.ListGroups(ctx, engine.Selector{InstallationID: "install"})
	if err != nil || len(listed) != 0 {
		t.Fatalf("list cleaned groups = %#v, %v", listed, err)
	}
}

func cloneResource(resource map[string]any) map[string]any {
	data, _ := json.Marshal(resource)
	var cloned map[string]any
	_ = json.Unmarshal(data, &cloned)
	return cloned
}

func TestKubernetesEngineRejectsInvalidConstructorConfiguration(t *testing.T) {
	base := EngineConfig{DataRoot: t.TempDir(), Namespace: "Orkest", InstallationID: "install", DataPVC: "task-data", UID: 1000, GID: 1000, Client: newFakeAPI()}
	if _, err := New(base); err == nil {
		t.Fatal("New accepted uppercase namespace")
	}
	base.Namespace = "orkest"
	base.DataSubpath = "../outside"
	if _, err := New(base); err == nil {
		t.Fatal("New accepted escaping PVC subpath")
	}
}

func TestKubernetesEngineRefusesExistingServiceWithForeignOwnership(t *testing.T) {
	root := t.TempDir()
	api := newFakeAPI()
	instance, err := New(EngineConfig{DataRoot: root, Namespace: "orkest", InstallationID: "install", DataPVC: "task-data", UID: 1000, GID: 1000, Client: api})
	if err != nil {
		t.Fatal(err)
	}
	plan := engine.WorkloadPlan{Group: engine.GroupIdentity{InstallationID: "install", Owner: "opaque-owner", Generation: "generation"}, Workloads: []engine.Workload{{ID: "process", Mode: engine.ExecutionModeContainer, Artifact: engine.Artifact{ContainerImage: "agent:test"}, Endpoints: []engine.Endpoint{{Name: "control", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: 8081, Exposure: engine.ExposurePrivate}}}}}
	name := infrastructurePodName(plan.Group)
	servicePath := instance.resourcePath("services", name)
	api.resources[servicePath] = map[string]any{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]any{"name": name, "namespace": "orkest", "labels": map[string]any{"app.kubernetes.io/managed-by": "other", "orkest.installation": "other"}},
	}
	if _, err = instance.StartPlan(context.Background(), plan); !errors.Is(err, engine.ErrIdentityMismatch) {
		t.Fatalf("StartPlan() with foreign Service ownership = %v", err)
	}
	if _, exists := api.resources[instance.resourcePath("pods", name)]; exists {
		t.Fatal("Kubernetes engine created a Pod after detecting foreign ownership")
	}
}
