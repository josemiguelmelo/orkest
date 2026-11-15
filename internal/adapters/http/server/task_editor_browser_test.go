//go:build docker

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sharededitor "github.com/josemiguelmelo/orkest/internal/adapters/executors/editor"
	dockerengine "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine/docker"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

type browserTasks struct{ gatewayTasks }

func (*browserTasks) Get(context.Context, string, string, string, string) (taskmodel.Task, error) {
	return taskmodel.Task{Repositories: []taskmodel.RepositoryRef{{ID: "repo", ProviderName: "github", Name: "repo"}}}, nil
}

type browserIdle struct{ busy atomic.Bool }

func (i *browserIdle) EditorIdle(context.Context, string, string, string) (bool, error) {
	return !i.busy.Load(), nil
}

func TestDockerEditorGatewayBrowser(t *testing.T) {
	if os.Getenv("TASK_EDITOR_BROWSER") == "" {
		t.Fatal("TASK_EDITOR_BROWSER is required for browser qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root := os.Getenv("ORKEST_DOCKER_TEST_DATA_ROOT")
	if root == "" {
		var err error
		root, err = filepath.Abs("../../../../tmp")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := os.MkdirTemp(root, "editor-gateway-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(data) }()
	work, _ := taskstorage.Directories(data, "org", "workspace", "task")
	repo := filepath.Join(work, "repositories", "github", "repo")
	if err = os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "git", "init", repo)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if err = filepath.WalkDir(data, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	engineClient, err := dockerengine.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	infrastructure, err := dockerengine.New(dockerengine.Config{DataRoot: data, Client: engineClient})
	if err != nil {
		_ = engineClient.Close()
		t.Fatal(err)
	}
	defer func() { _ = infrastructure.Close() }()
	uid, gid := uint32(os.Getuid()), uint32(os.Getgid())
	if uid == 0 || gid == 0 {
		uid, gid = 1000, 1000
	}
	backend, err := sharededitor.New(infrastructure, sharededitor.Config{Image: sharededitor.DefaultImage, ReadOnlyImage: sharededitor.DefaultReadOnlyImage, RunAsUser: uid, RunAsGroup: gid})
	if err != nil {
		t.Fatal(err)
	}
	idle := &browserIdle{}
	service, err := taskeditor.NewTaskEditorService(&browserTasks{}, backend, idle)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	scope := taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	defer func() {
		stopCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if err := service.ReconcileStartup(stopCtx); err != nil {
			t.Error(err)
		}
	}()
	var gateway *EditorGateway
	editor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gateway.ServeHTTP(w, r) }))
	defer editor.Close()
	editorOrigin := strings.Replace(editor.URL, "127.0.0.1", "editor.localhost", 1)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open":
			status, err := service.Open(r.Context(), "user", scope)
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			token, err := randomState()
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			gateway.mu.Lock()
			gateway.grants[token] = editorTicket{user: "user", scope: scope, generation: status.Generation, expires: time.Now().Add(time.Minute)}
			gateway.mu.Unlock()
			_ = json.NewEncoder(w).Encode(struct {
				taskeditor.Status
				LaunchURL string `json:"launchUrl"`
			}{status, editorOrigin + "/launch#" + token})
		case "/saved":
			data, err := os.ReadFile(filepath.Join(repo, "browser.txt"))
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(data)
		case "/transition":
			unlock, err := service.BeforeExecution(r.Context(), "org", "workspace", "task")
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			idle.busy.Store(true)
			unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer fixture.Close()
	gateway, err = NewEditorGateway(service, editorOrigin, fixture.URL)
	if err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../../../tools/task-editor/backend-browser.mjs")
	if err != nil {
		t.Fatal(err)
	}
	command = exec.CommandContext(ctx, "node", script)
	command.Env = append(os.Environ(), "ORKEST_EDITOR_FIXTURE="+fixture.URL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("backend browser: %v\n%s", err, output)
	}
	t.Log(string(output))
}
