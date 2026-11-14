package taskeditor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

type editorBackendFake struct {
	current       Instance
	found         bool
	starts, stops int
	stopError     error
}

func (b *editorBackendFake) StartEditor(_ context.Context, s Spec) (Instance, error) {
	b.starts++
	s.ID = fmt.Sprint(b.starts)
	s.Endpoint = "http://127.0.0.1:8080"
	b.current = s.Instance
	b.found = true
	return b.current, nil
}

func (b *editorBackendFake) InspectEditor(context.Context, Scope) (Instance, bool, error) {
	return b.current, b.found, nil
}

func (b *editorBackendFake) StopEditor(_ context.Context, i Instance) error {
	b.stops++
	if b.stopError != nil {
		return b.stopError
	}
	if i.ID != b.current.ID {
		return errors.New("stale stop")
	}
	b.found = false
	return nil
}

func (b *editorBackendFake) ListEditors(context.Context) ([]Instance, error) {
	if b.found {
		return []Instance{b.current}, nil
	}
	return nil, nil
}

type editorIdleFake struct {
	idle bool
	err  error
}

func (f *editorIdleFake) EditorIdle(context.Context, string, string, string) (bool, error) {
	return f.idle, f.err
}

func newEditorFixture(t *testing.T) (*TaskEditorService, *editorBackendFake, *editorIdleFake) {
	t.Helper()
	backend := &editorBackendFake{}
	idle := &editorIdleFake{idle: true}
	service, err := NewTaskEditorService(serviceTasks{}, &inspectorStub{inspection: Inspection{Roots: []Root{{Name: "repo", Path: "/workspace/repositories/github/repo", Source: "/data/repo"}}}}, backend, idle)
	if err != nil {
		t.Fatal(err)
	}
	return service, backend, idle
}

var editorTestScope = Scope{"org", "workspace", "task"}

func TestEditorHandoffRevokesGenerationAndPreservesRequest(t *testing.T) {
	s, b, idle := newEditorFixture(t)
	ctx := context.Background()
	opened, err := s.Open(ctx, "user", editorTestScope)
	if err != nil || opened.Mode != ReadWrite {
		t.Fatalf("open: %+v %v", opened, err)
	}
	_, lease, err := s.Access(ctx, "user", editorTestScope, opened.Generation)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := s.BeforeExecution(ctx, "org", "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	if b.found {
		t.Fatal("writable editor survived handoff")
	}
	select {
	case <-lease.Done():
	default:
		t.Fatal("old access not revoked")
	}
	idle.idle = false
	unlock()
	readonly, err := s.Open(ctx, "user", editorTestScope)
	if err != nil || readonly.Mode != ReadOnly || readonly.Generation == opened.Generation {
		t.Fatalf("readonly: %+v %v", readonly, err)
	}
	if _, _, err = s.Access(ctx, "user", editorTestScope, opened.Generation); !errors.Is(err, ErrGeneration) {
		t.Fatalf("stale access: %v", err)
	}
	idle.idle = true
	writable, err := s.Open(ctx, "user", editorTestScope)
	if err != nil || writable.Mode != ReadWrite {
		t.Fatalf("writable: %+v %v", writable, err)
	}
	if b.starts != 3 || b.stops != 2 {
		t.Fatalf("lifecycle counts: %+v", b)
	}
}

func TestEditorFailedStopBlocksDeliveryAndReplacement(t *testing.T) {
	s, b, idle := newEditorFixture(t)
	ctx := context.Background()
	if _, err := s.Open(ctx, "user", editorTestScope); err != nil {
		t.Fatal(err)
	}
	b.stopError = errors.New("uncertain termination")
	if unlock, err := s.BeforeExecution(ctx, "org", "workspace", "task"); err == nil {
		unlock()
		t.Fatal("handoff accepted uncertain stop")
	}
	idle.idle = false
	if _, err := s.Open(ctx, "user", editorTestScope); err == nil {
		t.Fatal("replacement accepted uncertain stop")
	}
	if b.starts != 1 {
		t.Fatal("replacement created")
	}
}

func TestEditorConcurrentOpenReusesOneInstance(t *testing.T) {
	s, b, _ := newEditorFixture(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := s.Open(context.Background(), "user", editorTestScope); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if b.starts != 1 {
		t.Fatalf("created %d instances", b.starts)
	}
	if _, err := s.Open(context.Background(), "other", editorTestScope); !errors.Is(err, ErrOwner) {
		t.Fatalf("other user: %v", err)
	}
}

func TestEditorGuardSerializesOpenWithDelivery(t *testing.T) {
	s, _, idle := newEditorFixture(t)
	unlock, err := s.BeforeExecution(context.Background(), "org", "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err = s.Open(ctx, "user", editorTestScope); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("open bypassed delivery lock: %v", err)
	}
	idle.idle = false
	unlock()
	status, err := s.Open(context.Background(), "user", editorTestScope)
	if err != nil || status.Mode != ReadOnly {
		t.Fatalf("pending execution: %+v %v", status, err)
	}
}

func TestEditorUncertainStateIsReadonlyAndStartupStopsResources(t *testing.T) {
	s, b, idle := newEditorFixture(t)
	idle.err = errors.New("database disconnected")
	status, err := s.Open(context.Background(), "user", editorTestScope)
	if err != nil || status.Mode != ReadOnly {
		t.Fatalf("uncertain state: %+v %v", status, err)
	}
	if err = s.ReconcileStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.found {
		t.Fatal("startup left stale editor")
	}
}

func TestEditorInspectionNeverStartsAndStopChecksOwner(t *testing.T) {
	s, b, _ := newEditorFixture(t)
	status, err := s.Inspect(context.Background(), "user", editorTestScope)
	if err != nil || status.State != "stopped" || b.starts != 0 {
		t.Fatalf("inspect: %+v %v", status, err)
	}
	if _, err = s.Open(context.Background(), "user", editorTestScope); err != nil {
		t.Fatal(err)
	}
	if err = s.Stop(context.Background(), "other", editorTestScope); !errors.Is(err, ErrOwner) {
		t.Fatalf("stop owner: %v", err)
	}
	if err = s.Stop(context.Background(), "user", editorTestScope); err != nil {
		t.Fatal(err)
	}
	if b.found {
		t.Fatal("stop left resource")
	}
}

type serviceTasks struct{}

func (serviceTasks) AuthorizeExecution(context.Context, string, string, string, string, bool) error {
	return nil
}

func (serviceTasks) Get(context.Context, string, string, string, string) (taskmodel.Task, error) {
	return taskmodel.Task{SourceProvider: "github", SourceID: "missing"}, nil
}

func TestEditorSelectionAndRemovalRevokeAccessBeforeMutation(t *testing.T) {
	s, b, _ := newEditorFixture(t)
	ctx := context.Background()
	status, err := s.Open(ctx, "user", editorTestScope)
	if err != nil {
		t.Fatal(err)
	}
	_, lease, err := s.Access(ctx, "user", editorTestScope, status.Generation)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := s.BeforeTaskMutation(ctx, "org", "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	if b.found || lease.Err() == nil {
		t.Fatal("selection mutation retained editor access")
	}
	unlock()
	if _, err = s.Open(ctx, "user", editorTestScope); err != nil {
		t.Fatal(err)
	}
	unlock, err = s.BeforeImportedTaskRemoval(ctx, "workspace", "github", nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.found {
		t.Fatal("import removal retained editor")
	}
	unlock()
}
