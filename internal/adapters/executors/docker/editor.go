package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	client "github.com/moby/moby/client"
)

const (
	editorLabel                = "io.orkest.editor"
	DefaultEditorImage         = "orkest/task-editor:4.145.2"
	DefaultReadonlyEditorImage = "orkest/task-editor:4.145.2-ro"
)

type editorEngine interface {
	EngineClient
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	NetworkCreate(context.Context, string, client.NetworkCreateOptions) (client.NetworkCreateResult, error)
	NetworkInspect(context.Context, string, client.NetworkInspectOptions) (client.NetworkInspectResult, error)
	NetworkConnect(context.Context, string, client.NetworkConnectOptions) (client.NetworkConnectResult, error)
	NetworkDisconnect(context.Context, string, client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error)
	NetworkRemove(context.Context, string, client.NetworkRemoveOptions) (client.NetworkRemoveResult, error)
	NetworkList(context.Context, client.NetworkListOptions) (client.NetworkListResult, error)
}

// EditorBackend shares the existing Engine client, task data root and networking.
// It never receives RunConfig, agent environment, credentials or host mounts.
type EditorBackend struct {
	starting                           sync.Map
	executor                           *Executor
	engine                             editorEngine
	installation, image, readonlyImage string
	user                               string
}

func (e *Executor) NewEditorBackend(image, readonlyImage string) (*EditorBackend, error) {
	engine, ok := e.client.(editorEngine)
	if !ok {
		return nil, errors.New("docker editor discovery requires container listing")
	}
	uidText, _, _ := strings.Cut(e.config.ContainerUser, ":")
	uid, uidErr := strconv.ParseUint(uidText, 10, 32)
	if uidErr != nil {
		return nil, errors.New("invalid editor container UID")
	}
	user := e.config.ContainerUser
	// An always-available editor capability must never inherit a root platform UID.
	// Root-owned checkouts still need operator-provided non-root access permissions.
	if uid == 0 {
		user = "1000:1000"
	}
	if strings.TrimSpace(image) == "" || strings.TrimSpace(readonlyImage) == "" || image == readonlyImage {
		return nil, errors.New("distinct editor and immutable readonly images are required")
	}
	hash := sha256.Sum256([]byte(e.config.DataRoot))
	return &EditorBackend{executor: e, engine: engine, installation: hex.EncodeToString(hash[:]), image: image, readonlyImage: readonlyImage, user: user}, nil
}

func (b *EditorBackend) name(scope taskeditor.Scope) string {
	data, _ := json.Marshal([]string{b.installation, scope.OrganizationID, scope.WorkspaceID, scope.TaskID})
	hash := sha256.Sum256(data)
	return "orkest-editor-" + hex.EncodeToString(hash[:])[:40]
}

func (b *EditorBackend) manifestDirectory(i taskeditor.Instance) (string, error) {
	if len(i.Generation) != 64 {
		return "", errors.New("invalid editor generation")
	}
	if _, err := hex.DecodeString(i.Generation); err != nil {
		return "", errors.New("invalid editor generation")
	}
	return filepath.Join(b.executor.config.DataRoot, "editor-workspaces", b.name(i.Scope), i.Generation), nil
}

func (b *EditorBackend) labels(i taskeditor.Instance) map[string]string {
	return map[string]string{editorLabel: b.installation, editorLabel + ".org": i.Scope.OrganizationID, editorLabel + ".workspace": i.Scope.WorkspaceID, editorLabel + ".task": i.Scope.TaskID, editorLabel + ".user": i.UserID, editorLabel + ".generation": i.Generation, editorLabel + ".mode": string(i.Mode), editorLabel + ".fingerprint": i.Fingerprint}
}

func (b *EditorBackend) decode(c container.InspectResponse) (taskeditor.Instance, error) {
	if c.Config == nil || c.Config.Labels[editorLabel] != b.installation {
		return taskeditor.Instance{}, errors.New("editor resource ownership mismatch")
	}
	l := c.Config.Labels
	i := taskeditor.Instance{ID: c.ID, Scope: taskeditor.Scope{OrganizationID: l[editorLabel+".org"], WorkspaceID: l[editorLabel+".workspace"], TaskID: l[editorLabel+".task"]}, UserID: l[editorLabel+".user"], Generation: l[editorLabel+".generation"], Mode: taskeditor.Mode(l[editorLabel+".mode"]), Fingerprint: l[editorLabel+".fingerprint"]}
	if i.Scope.OrganizationID == "" || i.Scope.WorkspaceID == "" || i.Scope.TaskID == "" || i.UserID == "" || len(i.Generation) != 64 || (i.Mode != taskeditor.ReadOnly && i.Mode != taskeditor.ReadWrite) {
		return i, errors.New("invalid editor resource metadata")
	}
	if _, err := hex.DecodeString(i.Generation); err != nil {
		return i, errors.New("invalid editor generation")
	}
	return i, nil
}

func (b *EditorBackend) InspectEditor(ctx context.Context, scope taskeditor.Scope) (taskeditor.Instance, bool, error) {
	result, err := b.engine.ContainerInspect(ctx, b.name(scope), client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return taskeditor.Instance{}, false, nil
	}
	if err != nil {
		return taskeditor.Instance{}, false, err
	}
	i, err := b.decode(result.Container)
	if err != nil {
		return i, false, err
	}
	if i.Scope != scope {
		return i, false, errors.New("editor scope mismatch")
	}
	if result.Container.State == nil {
		return i, false, errors.New("editor state unknown")
	}
	if result.Container.State.Running {
		if b.executor.config.NetworkName == "" {
			return b.inspectRelay(ctx, i)
		}
		i.Endpoint = "http://" + b.name(scope) + ":8080"
	}
	return i, true, nil
}

func (b *EditorBackend) StartEditor(ctx context.Context, spec taskeditor.Spec) (taskeditor.Instance, error) {
	// Re-resolve all mount sources immediately before creating the container.
	if spec.Mode != taskeditor.ReadOnly && spec.Mode != taskeditor.ReadWrite {
		return taskeditor.Instance{}, errors.New("invalid editor mode")
	}
	if len(spec.Discovery.Roots) != 1 || spec.Discovery.Roots[0].Path != "/task" {
		return taskeditor.Instance{}, taskeditor.ErrUnavailable
	}
	workRoot, rootErr := filepath.EvalSymlinks(b.executor.config.DataRoot)
	if rootErr != nil {
		return taskeditor.Instance{}, rootErr
	}
	taskWork, _ := taskstorage.Directories(workRoot, spec.Scope.OrganizationID, spec.Scope.WorkspaceID, spec.Scope.TaskID)
	if !filepath.IsAbs(taskWork) || !within(workRoot, taskWork) {
		return taskeditor.Instance{}, errors.New("invalid task workspace mount")
	}
	resolvedWork, err := existingDirectory(workRoot, taskWork)
	if err != nil || resolvedWork != taskWork {
		return taskeditor.Instance{}, errors.New("task workspace mount changed")
	}
	if err := b.requireProtectedWorkers(ctx, spec); err != nil {
		return taskeditor.Instance{}, err
	}
	manifestDir, err := b.manifestDirectory(spec.Instance)
	if err != nil {
		return taskeditor.Instance{}, err
	}
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		return taskeditor.Instance{}, err
	}
	manifest := filepath.Join(manifestDir, "task.code-workspace")
	manifestRetained := false
	defer func() {
		if !manifestRetained {
			_ = os.RemoveAll(manifestDir)
		}
	}()
	// The manifest contains only names/container paths. No host paths or secrets.
	if err = os.WriteFile(manifest, spec.Discovery.Manifest, 0o444); err != nil {
		return taskeditor.Instance{}, err
	}
	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: manifest, Target: "/workspace/task.code-workspace", ReadOnly: true},
		{Type: mount.TypeBind, Source: taskWork, Target: "/task", ReadOnly: spec.Mode == taskeditor.ReadOnly},
	}
	image := b.image
	if spec.Mode == taskeditor.ReadOnly {
		image = b.readonlyImage
	}
	port := network.MustParsePort("8080/tcp")
	pids := int64(256)
	host := &container.HostConfig{Mounts: mounts, ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}, Tmpfs: map[string]string{"/tmp": "rw,nosuid,nodev,size=256m,mode=1777", "/home/editor": "rw,nosuid,nodev,size=512m,mode=1777"}, Resources: container.Resources{Memory: 2 << 30, NanoCPUs: 2e9, PidsLimit: &pids}, RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled}}
	networkName := b.networkName(spec.Instance)
	if _, busy := b.starting.LoadOrStore(networkName, struct{}{}); busy {
		return taskeditor.Instance{}, taskeditor.ErrUnavailable
	}
	defer b.starting.Delete(networkName)
	containerCreated := false
	defer func() {
		if !containerCreated {
			b.cleanupUncreatedNetwork(ctx, spec.Instance)
			_ = os.RemoveAll(manifestDir)
		}
	}()
	if err = b.createNetwork(ctx, spec.Instance); err != nil {
		return spec.Instance, err
	}
	host.NetworkMode = container.NetworkMode(networkName)

	created, err := b.engine.ContainerCreate(ctx, client.ContainerCreateOptions{Name: b.name(spec.Scope), Config: &container.Config{Image: image, User: b.user, Env: []string{"HOME=/home/editor", "EDITOR_MODE=" + string(spec.Mode)}, Labels: b.labels(spec.Instance), ExposedPorts: network.PortSet{port: {}}}, HostConfig: host})
	if err != nil {
		return taskeditor.Instance{}, fmt.Errorf("create editor: %w", err)
	}
	containerCreated = true
	manifestRetained = true
	spec.ID = created.ID
	if _, err = b.engine.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return spec.Instance, fmt.Errorf("start editor: %w", err)
	}
	if b.executor.config.NetworkName == "" {
		if err = b.startRelay(ctx, spec.Instance); err != nil {
			return spec.Instance, err
		}
	}
	i, found, err := b.InspectEditor(ctx, spec.Scope)
	if err != nil {
		return spec.Instance, err
	}
	if !found || i.ID != created.ID || i.Endpoint == "" {
		return spec.Instance, errors.New("editor failed to start")
	}
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	httpClient := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		request, reqErr := http.NewRequestWithContext(readyCtx, http.MethodGet, i.Endpoint+"/healthz", nil)
		if reqErr != nil {
			return i, reqErr
		}
		if i.GatewayToken != "" {
			request.Header.Set("X-Orkest-Editor-Relay", i.GatewayToken)
		}
		response, requestErr := httpClient.Do(request)
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return i, nil
			}
		}
		select {
		case <-readyCtx.Done():
			return i, errors.New("editor readiness timed out")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (b *EditorBackend) StopEditor(ctx context.Context, i taskeditor.Instance) error {
	if i.ID == "" {
		return errors.New("concrete editor ID required")
	}
	if err := b.stopRelay(ctx, i); err != nil {
		return err
	}
	result, err := b.engine.ContainerInspect(ctx, i.ID, client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		if err := b.removeNetwork(ctx, i); err != nil {
			return err
		}
		return b.removeManifest(i)
	}
	if err != nil {
		return err
	}
	actual, err := b.decode(result.Container)
	if err != nil {
		return err
	}
	if actual.Scope != i.Scope || actual.Generation != i.Generation || actual.UserID != i.UserID {
		return errors.New("editor stop identity mismatch")
	}
	timeout := 5
	if _, err = b.engine.ContainerStop(ctx, i.ID, client.ContainerStopOptions{Timeout: &timeout}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	result, err = b.engine.ContainerInspect(ctx, i.ID, client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		if err := b.removeNetwork(ctx, i); err != nil {
			return err
		}
		return b.removeManifest(i)
	}
	if err != nil {
		return err
	}
	if result.Container.State == nil || result.Container.State.Running {
		return errors.New("editor termination unconfirmed")
	}
	_, err = b.engine.ContainerRemove(ctx, i.ID, client.ContainerRemoveOptions{})
	if errdefs.IsNotFound(err) {
		if err := b.removeNetwork(ctx, i); err != nil {
			return err
		}
		return b.removeManifest(i)
	}
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	if err = b.removeNetwork(ctx, i); err != nil {
		return err
	}
	return b.removeManifest(i)
}

func (b *EditorBackend) removeManifest(i taskeditor.Instance) error {
	directory, err := b.manifestDirectory(i)
	if err != nil {
		return err
	}
	return os.RemoveAll(directory)
}

func (b *EditorBackend) ListEditors(ctx context.Context) ([]taskeditor.Instance, error) {
	listed, err := b.engine.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("label", editorLabel+"="+b.installation)})
	if err != nil {
		return nil, err
	}
	result := make([]taskeditor.Instance, 0, len(listed.Items))
	for _, summary := range listed.Items {
		inspection, err := b.engine.ContainerInspect(ctx, summary.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if inspection.Container.Config != nil && inspection.Container.Config.Labels[editorLabel+".kind"] == "relay" {
			continue
		}
		i, err := b.decode(inspection.Container)
		if err != nil {
			return nil, err
		}
		result = append(result, i)
	}
	if err = b.cleanupOrphanRelays(ctx, listed.Items, result); err != nil {
		return nil, err
	}
	if err = b.cleanupOrphanNetworks(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}
