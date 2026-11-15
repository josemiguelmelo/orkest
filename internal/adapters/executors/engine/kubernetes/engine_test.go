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
	if method == http.MethodGet && requestPath == "/api/v1/namespaces/orkest/pods" {
		items := make([]any, 0)
		for path, resource := range f.resources {
			if len(path) > len(requestPath) && path[:len(requestPath)+1] == requestPath+"/" && path[len(requestPath)+1:len(requestPath)+7] == "orkest" {
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

func TestKubernetesCreateFailureRetainsIdentityForReconciliation(t *testing.T) {
	root := t.TempDir()
	api := newFakeAPI()
	workspaces, err := NewWorkspaceResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	scope := engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	workspace, err := workspaces.ResolveWorkspace(context.Background(), engine.WorkspaceRequest{Scope: scope, Intent: engine.WorkspaceEnsureForAgent})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := New(EngineConfig{DataRoot: root, Namespace: "orkest", InstallationID: "install", DataPVC: "task-data", UID: 1000, GID: 1000, Client: api})
	if err != nil {
		t.Fatal(err)
	}
	key := engine.ResourceKey{InstallationID: "install", Scope: scope, Class: engine.ResourceClassAgent, ExecutionID: "run-create-failure", Qualifier: "opencode"}
	spec := engine.LaunchSpec{
		Key: key, Class: engine.ResourceClassAgent, Mode: engine.ExecutionModeContainer,
		Artifact: engine.Artifact{ContainerImage: "agent:test"}, Workspace: workspace,
		Endpoints:    []engine.Endpoint{{Name: "agentctl", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: agentctlPort, Ingress: engine.IngressWorkerControl}},
		Requirements: engine.Requirements{Mode: engine.ExecutionModeContainer, DurableWork: true, DurableState: true},
	}
	api.failCreateKind = "Pod"
	resource, err := instance.Start(context.Background(), spec)
	if err == nil || resource.Ref.ConcreteID == "" {
		t.Fatalf("Start() failure did not preserve reconciliation identity: %+v, %v", resource, err)
	}
	name, _, parseErr := instance.parseID(resource.Ref.ConcreteID)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	servicePath := instance.resourcePath("services", name)
	if api.resources[servicePath] == nil {
		t.Fatal("partial create fixture did not retain the Service")
	}
	api.failCreateKind = ""
	if err := instance.Cleanup(context.Background(), resource.Ref); err != nil {
		t.Fatal(err)
	}
	if api.resources[servicePath] != nil {
		t.Fatal("reconciliation cleanup left the owned Service")
	}
}

func cloneResource(resource map[string]any) map[string]any {
	data, _ := json.Marshal(resource)
	var cloned map[string]any
	_ = json.Unmarshal(data, &cloned)
	return cloned
}

func TestKubernetesEngineStartsInspectsAndStopsOwnedPod(t *testing.T) {
	root := t.TempDir()
	api := newFakeAPI()
	workspaces, err := NewWorkspaceResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	key := engine.ResourceKey{InstallationID: "install", Scope: engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}, Class: engine.ResourceClassAgent, ExecutionID: "run-1", Qualifier: "opencode"}
	workspace, err := workspaces.ResolveWorkspace(context.Background(), engine.WorkspaceRequest{Scope: key.Scope, Intent: engine.WorkspaceEnsureForAgent})
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(EngineConfig{DataRoot: root, Namespace: "orkest", InstallationID: "install", DataPVC: "task-data", CodexAuthSecret: "codex-auth", CodexAuthSecretKey: "auth.json", UID: 1000, GID: 1000, Client: api})
	if err != nil {
		t.Fatal(err)
	}
	spec := engine.LaunchSpec{
		Key: key, Class: engine.ResourceClassAgent, Mode: engine.ExecutionModeContainer,
		Artifact:           engine.Artifact{ContainerImage: "agent:test"},
		Arguments:          []engine.Argument{{Kind: engine.ArgumentLiteral, Value: "-listen"}, {Kind: engine.ArgumentEndpointAddress, Endpoint: "agentctl"}},
		Environment:        map[string]string{"TOKEN": "sensitive-value", "ORKEST_CODEX_AUTH_FILE": "/agentctl-state/codex-auth/auth.json"},
		CredentialBindings: []engine.CredentialBinding{{Name: "codex-auth", Target: "/agentctl-state/codex-auth/auth.json", ReadOnly: true}},
		Workspace:          workspace,
		Endpoints:          []engine.Endpoint{{Name: "agentctl", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: agentctlPort, Ingress: engine.IngressWorkerControl}},
		Requirements:       engine.Requirements{Mode: engine.ExecutionModeContainer, DurableWork: true, DurableState: true, CredentialBindings: true},
	}
	resource, err := client.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if resource.State != engine.ResourceRunning || resource.Endpoint == "" || resource.Ref.ConcreteID == "" {
		t.Fatalf("started resource = %#v", resource)
	}
	name, _, err := client.parseID(resource.Ref.ConcreteID)
	if err != nil {
		t.Fatal(err)
	}
	pod := api.resources[client.resourcePath("pods", name)]
	if pod == nil {
		t.Fatal("Kubernetes engine did not create the pod")
	}
	encoded, _ := json.Marshal(pod)
	if string(encoded) == "" || strings.Contains(string(encoded), "sensitive-value") {
		t.Fatalf("Pod resource leaked secret value: %s", encoded)
	}
	if !strings.Contains(string(encoded), "tasks/"+workspace.CompatibilityID) {
		t.Fatalf("Pod does not mount the compatible task subpath: %s", encoded)
	}
	podSpec, _ := pod["spec"].(map[string]any)
	if podSpec["restartPolicy"] != "Never" || podSpec["automountServiceAccountToken"] != false {
		t.Fatalf("Pod restart/token policy = %#v / %#v", podSpec["restartPolicy"], podSpec["automountServiceAccountToken"])
	}
	containers, _ := podSpec["containers"].([]any)
	containerSpec, _ := containers[0].(map[string]any)
	environment, _ := containerSpec["env"].([]any)
	if countEnvironment(environment, "ORKEST_CODEX_AUTH_FILE") != 1 || countEnvironment(environment, "TOKEN") != 1 {
		t.Fatalf("Pod environment entries = %#v", environment)
	}
	secret := api.resources[client.resourcePath("secrets", name+"-env")]
	secretData, _ := secret["stringData"].(map[string]any)
	if secretData["TOKEN"] != "sensitive-value" || secretData["ORKEST_CODEX_AUTH_FILE"] != nil {
		t.Fatalf("generated Secret values = %#v", secretData)
	}
	observed, err := client.Inspect(context.Background(), engine.Lookup{Ref: resource.Ref})
	if err != nil || observed.State != engine.ObservationActive {
		t.Fatalf("Inspect() = %#v, %v", observed, err)
	}
	wrongRun := resource.Ref
	wrongRun.Key.ExecutionID = "another-run"
	if err = client.Stop(context.Background(), wrongRun); err != engine.ErrIdentityMismatch {
		t.Fatalf("Stop with mismatched run identity = %v", err)
	}
	wrongUID := resource.Ref
	wrongUID.ConcreteID = client.processID(name, "different-uid")
	if err = client.Stop(context.Background(), wrongUID); err != engine.ErrIdentityMismatch {
		t.Fatalf("Stop with mismatched Pod UID = %v", err)
	}
	if err = client.Stop(context.Background(), resource.Ref); err != nil {
		t.Fatal(err)
	}
	observed, err = client.Inspect(context.Background(), engine.Lookup{Ref: resource.Ref})
	if err != nil || observed.State != engine.ObservationAbsent {
		t.Fatalf("Inspect() after Stop = %#v, %v", observed, err)
	}
}

func countEnvironment(values []any, name string) int {
	count := 0
	for _, value := range values {
		entry, _ := value.(map[string]any)
		if entry["name"] == name {
			count++
		}
	}
	return count
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
	workspaces, err := NewWorkspaceResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	key := engine.ResourceKey{InstallationID: "install", Scope: engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}, Class: engine.ResourceClassAgent, ExecutionID: "run-conflict", Qualifier: "opencode"}
	workspace, err := workspaces.ResolveWorkspace(context.Background(), engine.WorkspaceRequest{Scope: key.Scope, Intent: engine.WorkspaceEnsureForAgent})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := New(EngineConfig{DataRoot: root, Namespace: "orkest", InstallationID: "install", DataPVC: "task-data", UID: 1000, GID: 1000, Client: api})
	if err != nil {
		t.Fatal(err)
	}
	name := resourceName("install", key.ExecutionID)
	servicePath := instance.resourcePath("services", name)
	api.resources[servicePath] = map[string]any{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]any{"name": name, "namespace": "orkest", "labels": map[string]any{"app.kubernetes.io/managed-by": "other", "orkest.installation": "other"}},
	}
	spec := engine.LaunchSpec{
		Key: key, Class: engine.ResourceClassAgent, Mode: engine.ExecutionModeContainer,
		Artifact: engine.Artifact{ContainerImage: "agent:test"}, Workspace: workspace,
		Endpoints:    []engine.Endpoint{{Name: "agentctl", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: agentctlPort, Ingress: engine.IngressWorkerControl}},
		Requirements: engine.Requirements{Mode: engine.ExecutionModeContainer, DurableWork: true, DurableState: true},
	}
	if _, err = instance.Start(context.Background(), spec); !errors.Is(err, engine.ErrIdentityMismatch) {
		t.Fatalf("Start() with foreign Service ownership = %v", err)
	}
	if _, exists := api.resources[instance.resourcePath("pods", name)]; exists {
		t.Fatal("Kubernetes engine created a Pod after detecting foreign ownership")
	}
}
