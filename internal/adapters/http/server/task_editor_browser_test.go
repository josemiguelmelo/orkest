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
	"sync/atomic"
	"testing"
	"time"

	dockerexecutor "github.com/josemiguelmelo/orkest/internal/adapters/executors/docker"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

type browserTasks struct{ gatewayTasks }

func (browserTasks) Get(context.Context, string, string, string, string) (taskmodel.Task, error) {
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
	root, err := filepath.Abs("../../../../tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0o700); err != nil {
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
	executor, err := dockerexecutor.New(ctx, dockerexecutor.Config{DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = executor.Close() }()
	backend, err := executor.NewEditorBackend("orkest/task-editor-spike:4.145.2", "orkest/task-editor-spike:4.145.2-ro")
	if err != nil {
		t.Fatal(err)
	}
	idle := &browserIdle{}
	service, err := taskeditor.NewTaskEditorService(&browserTasks{}, dockerexecutor.TaskCheckoutInspector{DataRoot: data}, backend, idle)
	if err != nil {
		t.Fatal(err)
	}
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
			}{status, editor.URL + "/launch#" + token})
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
	gateway, err = NewEditorGateway(service, editor.URL, fixture.URL)
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
