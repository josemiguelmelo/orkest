//go:build docker

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sharededitor "github.com/josemiguelmelo/orkest/internal/adapters/executors/editor"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	dockerengine "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine/docker"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	mobyclient "github.com/moby/moby/client"
)

func TestDockerEngineSharedEditorLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var err error
	dataParent := os.Getenv("ORKEST_DOCKER_TEST_DATA_ROOT")
	if dataParent == "" {
		dataParent, err = filepath.Abs("../../tmp")
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(dataParent, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(dataParent, "executor-engine-editor-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if removeErr := os.RemoveAll(root); removeErr != nil {
			t.Errorf("remove editor integration data: %v", removeErr)
		}
	}()
	client, err := dockerengine.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	infrastructure, err := dockerengine.New(dockerengine.Config{DataRoot: root, Client: client})
	if err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	defer func() { _ = infrastructure.Close() }()
	uid, gid := uint32(os.Getuid()), uint32(os.Getgid())
	if uid == 0 || gid == 0 {
		uid, gid = 1000, 1000
	}
	image := os.Getenv("ORKEST_EDITOR_IMAGE")
	if image == "" {
		image = sharededitor.DefaultImage
	}
	readOnlyImage := os.Getenv("ORKEST_EDITOR_READONLY_IMAGE")
	if readOnlyImage == "" {
		readOnlyImage = sharededitor.DefaultReadOnlyImage
	}
	executor, err := sharededitor.New(infrastructure, sharededitor.Config{Image: image, ReadOnlyImage: readOnlyImage, RunAsUser: uid, RunAsGroup: gid})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		instances, listErr := executor.ListEditors(cleanupCtx)
		if listErr != nil {
			t.Error(listErr)
			return
		}
		for _, instance := range instances {
			if stopErr := executor.StopEditor(cleanupCtx, instance); stopErr != nil {
				t.Error(stopErr)
			}
		}
	}()
	scope := taskeditor.Scope{OrganizationID: "engine-integration", WorkspaceID: "docker-editor", TaskID: "task"}
	resolved, err := infrastructure.ResolveWorkspace(ctx, engine.WorkspaceRequest{Scope: engine.Scope{OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID}, Intent: engine.WorkspaceEnsureForAgent})
	if err != nil {
		t.Fatal(err)
	}
	if err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for index, mode := range []taskeditor.Mode{taskeditor.ReadWrite, taskeditor.ReadOnly} {
		inspection, inspectErr := executor.Inspect(ctx, scope.OrganizationID, scope.WorkspaceID, scope.TaskID, nil)
		if inspectErr != nil || len(inspection.Roots) != 1 {
			t.Fatalf("Inspect() = %#v, %v", inspection, inspectErr)
		}
		generation := strings.Repeat(fmt.Sprintf("%x", index+1), 64)
		manifest, marshalErr := json.Marshal(struct {
			Folders []taskeditor.Root `json:"folders"`
		}{inspection.Roots})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		instance, startErr := executor.StartEditor(ctx, taskeditor.Spec{
			Instance:  taskeditor.Instance{Scope: scope, UserID: "engine-test", Generation: generation, Fingerprint: "integration", Mode: mode},
			Discovery: taskeditor.Discovery{CompatibilityKey: inspection.CompatibilityKey, Roots: inspection.Roots, Manifest: manifest},
		})
		if startErr != nil {
			t.Fatal(startErr)
		}
		if mode == taskeditor.ReadWrite {
			command := exec.CommandContext(ctx, "docker", "exec", instance.ID, "bash", "-c", "echo saved > /task/saved.txt")
			if output, writeErr := command.CombinedOutput(); writeErr != nil {
				t.Fatalf("editor write: %v: %s", writeErr, output)
			}
		} else {
			command := exec.CommandContext(ctx, "docker", "exec", instance.ID, "bash", "-c", "echo denied > /task/denied.txt")
			if output, writeErr := command.CombinedOutput(); writeErr == nil {
				t.Fatalf("read-only editor accepted a write: %s", output)
			}
		}
		if stopErr := executor.StopEditor(ctx, instance); stopErr != nil {
			t.Fatal(stopErr)
		}
	}
	if saved, readErr := os.ReadFile(filepath.Join(resolved.WorkPath, "saved.txt")); readErr != nil || string(saved) != "saved\n" {
		t.Fatalf("saved workspace file = %q, %v", saved, readErr)
	}
}

func TestDockerEngineEditorsUsePrivateNetworksAndRelayTokens(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dataParent := os.Getenv("ORKEST_DOCKER_TEST_DATA_ROOT")
	if dataParent == "" {
		var err error
		dataParent, err = filepath.Abs("../../tmp")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dataParent, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(dataParent, "executor-engine-editor-security-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	client, err := dockerengine.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	infrastructure, err := dockerengine.New(dockerengine.Config{DataRoot: root, Client: client})
	if err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	defer func() { _ = infrastructure.Close() }()
	uid, gid := uint32(os.Getuid()), uint32(os.Getgid())
	if uid == 0 || gid == 0 {
		uid, gid = 1000, 1000
	}
	image, readOnlyImage := sharededitor.DefaultImage, sharededitor.DefaultReadOnlyImage
	if value := os.Getenv("ORKEST_EDITOR_IMAGE"); value != "" {
		image = value
	}
	if value := os.Getenv("ORKEST_EDITOR_READONLY_IMAGE"); value != "" {
		readOnlyImage = value
	}
	executor, err := sharededitor.New(infrastructure, sharededitor.Config{Image: image, ReadOnlyImage: readOnlyImage, RunAsUser: uid, RunAsGroup: gid})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		instances, listErr := executor.ListEditors(cleanupCtx)
		if listErr != nil {
			t.Error(listErr)
			return
		}
		for _, instance := range instances {
			if stopErr := executor.StopEditor(cleanupCtx, instance); stopErr != nil {
				t.Error(stopErr)
			}
		}
	}()
	instances := make([]taskeditor.Instance, 0, 2)
	for _, workspaceID := range []string{"workspace-a", "workspace-b"} {
		scope := taskeditor.Scope{OrganizationID: "engine-security", WorkspaceID: workspaceID, TaskID: "task"}
		resolved, resolveErr := infrastructure.ResolveWorkspace(ctx, engine.WorkspaceRequest{Scope: engine.Scope{OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID}, Intent: engine.WorkspaceEnsureForAgent})
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		if walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return os.Chmod(path, 0o755)
			}
			return nil
		}); walkErr != nil {
			t.Fatal(walkErr)
		}
		inspection, inspectErr := executor.Inspect(ctx, scope.OrganizationID, scope.WorkspaceID, scope.TaskID, nil)
		if inspectErr != nil || len(inspection.Roots) != 1 {
			t.Fatalf("Inspect(%s) = %#v, %v", workspaceID, inspection, inspectErr)
		}
		manifest, marshalErr := json.Marshal(struct {
			Folders []taskeditor.Root `json:"folders"`
		}{inspection.Roots})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		instance, startErr := executor.StartEditor(ctx, taskeditor.Spec{
			Instance: taskeditor.Instance{Scope: scope, UserID: workspaceID, Generation: strings.Repeat("c", 64), Fingerprint: "network-isolation", Mode: taskeditor.ReadOnly},
			Discovery: taskeditor.Discovery{CompatibilityKey: inspection.CompatibilityKey, Roots: inspection.Roots, Manifest: manifest},
		})
		if startErr != nil {
			t.Fatal(startErr)
		}
		containerResult, inspectErr := client.ContainerInspect(ctx, instance.ID, mobyclient.ContainerInspectOptions{})
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
		if containerResult.Container.NetworkSettings == nil || len(containerResult.Container.NetworkSettings.Networks) != 1 {
			t.Fatalf("editor networks = %#v", containerResult.Container.NetworkSettings)
		}
		for networkName := range containerResult.Container.NetworkSettings.Networks {
			privateNetwork, networkErr := client.NetworkInspect(ctx, networkName, mobyclient.NetworkInspectOptions{})
			if networkErr != nil || !privateNetwork.Network.Internal {
				t.Fatalf("editor private network %q = %#v, %v", networkName, privateNetwork.Network, networkErr)
			}
		}
		_ = resolved
		instances = append(instances, instance)
	}
	if instances[0].Endpoint == instances[1].Endpoint || instances[0].GatewayToken == instances[1].GatewayToken {
		t.Fatal("editor generations do not have separate relay endpoints and tokens")
	}
	probe := func(endpoint, token string) int {
		t.Helper()
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/healthz", nil)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if token != "" {
			request.Header.Set("X-Orkest-Editor-Relay", token)
		}
		response, doErr := (&http.Client{Timeout: 3 * time.Second}).Do(request)
		if doErr != nil {
			t.Fatal(doErr)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	if status := probe(instances[0].Endpoint, ""); status != http.StatusForbidden {
		t.Fatalf("unauthenticated relay status = %d", status)
	}
	if status := probe(instances[0].Endpoint, instances[1].GatewayToken); status != http.StatusForbidden {
		t.Fatalf("cross-editor relay token status = %d", status)
	}
	if status := probe(instances[0].Endpoint, instances[0].GatewayToken); status != http.StatusOK {
		t.Fatalf("own relay token status = %d", status)
	}
}
