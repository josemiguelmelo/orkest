package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	client "github.com/moby/moby/client"
)

const editorLabel = "io.orkest.editor"

type editorEngine interface {
	EngineClient
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
}

// EditorBackend shares the existing Engine client, task data root and networking.
// It never receives RunConfig, agent environment, credentials or host mounts.
type EditorBackend struct {
	executor                           *Executor
	engine                             editorEngine
	installation, image, readonlyImage string
}

func (e *Executor) NewEditorBackend(image, readonlyImage string) (*EditorBackend, error) {
	engine, ok := e.client.(editorEngine)
	if !ok {
		return nil, errors.New("docker editor discovery requires container listing")
	}
	if strings.HasPrefix(e.config.ContainerUser, "0:") {
		return nil, errors.New("editor requires non-root container UID")
	}
	if strings.TrimSpace(image) == "" || strings.TrimSpace(readonlyImage) == "" || image == readonlyImage {
		return nil, errors.New("distinct editor and immutable readonly images are required")
	}
	hash := sha256.Sum256([]byte(e.config.DataRoot))
	return &EditorBackend{executor: e, engine: engine, installation: hex.EncodeToString(hash[:]), image: image, readonlyImage: readonlyImage}, nil
}

func (b *EditorBackend) name(scope taskeditor.Scope) string {
	data, _ := json.Marshal([]string{b.installation, scope.OrganizationID, scope.WorkspaceID, scope.TaskID})
	hash := sha256.Sum256(data)
	return "orkest-editor-" + hex.EncodeToString(hash[:])[:40]
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
		i.Endpoint, err = publishedEndpoint(result.Container, 8080)
		if b.executor.config.NetworkName != "" {
			i.Endpoint = "http://" + b.name(scope) + ":8080"
			err = nil
		}
		if err != nil {
			return i, false, err
		}
	}
	return i, true, nil
}

func (b *EditorBackend) StartEditor(ctx context.Context, spec taskeditor.Spec) (taskeditor.Instance, error) {
	// Re-resolve all mount sources immediately before creating the container.
	if spec.Mode != taskeditor.ReadOnly && spec.Mode != taskeditor.ReadWrite {
		return taskeditor.Instance{}, errors.New("invalid editor mode")
	}
	if len(spec.Discovery.Roots) == 0 {
		return taskeditor.Instance{}, taskeditor.ErrUnavailable
	}
	workRoot, rootErr := filepath.EvalSymlinks(b.executor.config.DataRoot)
	if rootErr != nil {
		return taskeditor.Instance{}, rootErr
	}
	for _, root := range spec.Discovery.Roots {
		if !filepath.IsAbs(root.Source) || !within(workRoot, root.Source) || !strings.HasPrefix(root.Path, "/workspace/repositories/") {
			return taskeditor.Instance{}, errors.New("invalid editor mount")
		}
		resolved, err := existingDirectory(workRoot, root.Source)
		if err != nil || resolved != root.Source {
			return taskeditor.Instance{}, errors.New("editor mount changed")
		}
		if err = validateGitMetadata(ctx, filepath.Join(root.Source, ".git")); err != nil {
			return taskeditor.Instance{}, err
		}
	}
	manifestRoot := filepath.Join(b.executor.config.DataRoot, "editor-workspaces")
	if err := os.MkdirAll(manifestRoot, 0o700); err != nil {
		return taskeditor.Instance{}, err
	}
	manifest, cleanup, err := taskeditor.MaterializeManifestAt(manifestRoot, spec.Discovery.Manifest)
	if err != nil {
		return taskeditor.Instance{}, err
	}
	defer cleanup()
	// The manifest contains only names/container paths. No host paths or secrets.
	if err = os.Chmod(manifest, 0o444); err != nil {
		return taskeditor.Instance{}, err
	}
	mounts := []mount.Mount{{Type: mount.TypeBind, Source: manifest, Target: "/workspace/task.code-workspace", ReadOnly: true}}
	for _, root := range spec.Discovery.Roots {
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: root.Source, Target: root.Path, ReadOnly: spec.Mode == taskeditor.ReadOnly})
	}
	image := b.image
	if spec.Mode == taskeditor.ReadOnly {
		image = b.readonlyImage
	}
	port := network.MustParsePort("8080/tcp")
	pids := int64(256)
	host := &container.HostConfig{Mounts: mounts, ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}, Tmpfs: map[string]string{"/tmp": "rw,nosuid,nodev,size=256m,mode=1777", "/home/editor": "rw,nosuid,nodev,size=512m,mode=1777"}, Resources: container.Resources{Memory: 2 << 30, NanoCPUs: 2e9, PidsLimit: &pids}, PortBindings: network.PortMap{port: {{HostIP: netip.MustParseAddr("127.0.0.1")}}}, RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled}}
	if b.executor.config.NetworkName != "" {
		host.NetworkMode = container.NetworkMode(b.executor.config.NetworkName)
		host.PortBindings = nil
	}
	created, err := b.engine.ContainerCreate(ctx, client.ContainerCreateOptions{Name: b.name(spec.Scope), Config: &container.Config{Image: image, User: b.executor.config.ContainerUser, Env: []string{"HOME=/home/editor", "EDITOR_MODE=" + string(spec.Mode)}, Labels: b.labels(spec.Instance), ExposedPorts: network.PortSet{port: {}}}, HostConfig: host})
	if err != nil {
		return taskeditor.Instance{}, fmt.Errorf("create editor: %w", err)
	}
	spec.ID = created.ID
	if _, err = b.engine.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return spec.Instance, fmt.Errorf("start editor: %w", err)
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
	result, err := b.engine.ContainerInspect(ctx, i.ID, client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
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
		return nil
	}
	if err != nil {
		return err
	}
	if result.Container.State == nil || result.Container.State.Running {
		return errors.New("editor termination unconfirmed")
	}
	_, err = b.engine.ContainerRemove(ctx, i.ID, client.ContainerRemoveOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	return err
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
		i, err := b.decode(inspection.Container)
		if err != nil {
			return nil, err
		}
		result = append(result, i)
	}
	return result, nil
}
