package taskeditor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

type editorBackendFake struct {
	CheckoutInspector
	current       Instance
	found         bool
	starts, stops int
	stopError     error
	inspectError  error
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
	return b.current, b.found, b.inspectError
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
	backend := &editorBackendFake{CheckoutInspector: &inspectorStub{inspection: Inspection{Roots: []Root{{Name: "repo", Path: "/workspace/repositories/github/repo", StorageRef: "/data/repo"}}}}}
	idle := &editorIdleFake{idle: true}
	service, err := NewTaskEditorService(serviceTasks{}, backend, idle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	return service, backend, idle
}

var editorTestScope = Scope{"org", "workspace", "task"}

func TestEditorReusesOnlyCompatibleWorkspace(t *testing.T) {
	s, backend, _ := newEditorFixture(t)
	ctx := context.Background()
	first, err := s.Open(ctx, "user", editorTestScope)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Open(ctx, "user", editorTestScope)
	if err != nil || again.Generation != first.Generation || backend.starts != 1 {
		t.Fatalf("compatible open recreated editor: %+v %v", again, err)
	}
	inspection := backend.CheckoutInspector.(*inspectorStub)
	for _, change := range []func(){
		func() { inspection.inspection.Roots[0].StorageRef = "/data/replacement" },
		func() { inspection.inspection.CompatibilityKey = "changed-adapter-config" },
	} {
		_, lease, err := s.Access(ctx, "user", editorTestScope, again.Generation)
		if err != nil {
			t.Fatal(err)
		}
		change()
		next, err := s.Open(ctx, "user", editorTestScope)
		if err != nil {
			t.Fatal(err)
		}
		if next.Generation == again.Generation || lease.Err() == nil {
			t.Fatal("changed workspace retained old generation access")
		}
		again = next
	}
}

func TestWorkspaceFingerprintPreservesReferenceBoundaries(t *testing.T) {
	first := Discovery{Roots: []Root{{Name: "a", Path: "/a", StorageRef: "ab"}, {Name: "b", Path: "/b", StorageRef: "c"}}}
	second := Discovery{Roots: []Root{{Name: "a", Path: "/a", StorageRef: "a"}, {Name: "b", Path: "/b", StorageRef: "bc"}}}
	if workspaceFingerprint(first, ReadOnly) == workspaceFingerprint(second, ReadOnly) {
		t.Fatal("ambiguous storage references shared compatibility fingerprint")
	}
	if workspaceFingerprint(first, ReadOnly) == workspaceFingerprint(first, ReadWrite) {
		t.Fatal("different modes shared compatibility fingerprint")
	}
}

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

func TestEditorOwnerSurvivesHandoffGap(t *testing.T) {
	s, _, _ := newEditorFixture(t)
	ctx := context.Background()
	if _, err := s.Open(ctx, "owner", editorTestScope); err != nil {
		t.Fatal(err)
	}
	unlock, err := s.BeforeExecution(ctx, "org", "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if _, err = s.Open(ctx, "other", editorTestScope); !errors.Is(err, ErrOwner) {
		t.Fatalf("ownership lost while replacement pending: %v", err)
	}
	if err = s.Stop(ctx, "other", editorTestScope); !errors.Is(err, ErrOwner) {
		t.Fatalf("other user stopped pending editor: %v", err)
	}
}

func TestSuccessfulStopClearsEarlierFailure(t *testing.T) {
	s, b, idle := newEditorFixture(t)
	ctx := context.Background()
	if _, err := s.Open(ctx, "owner", editorTestScope); err != nil {
		t.Fatal(err)
	}
	b.stopError = errors.New("uncertain stop")
	idle.idle = false
	if _, err := s.Open(ctx, "owner", editorTestScope); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := s.Inspect(ctx, "other", editorTestScope); !errors.Is(err, ErrOwner) {
		t.Fatalf("failed state bypassed owner: %v", err)
	}
	b.stopError = nil
	if err := s.Stop(ctx, "owner", editorTestScope); err != nil {
		t.Fatal(err)
	}
	status, err := s.Inspect(ctx, "owner", editorTestScope)
	if err != nil || status.State != "stopped" {
		t.Fatalf("stale failure after stop: %+v %v", status, err)
	}
}

type revokedEditorTasks struct {
	serviceTasks
	denied atomic.Bool
}

func (t *revokedEditorTasks) AuthorizeExecution(context.Context, string, string, string, string, bool) error {
	if t.denied.Load() {
		return errors.New("membership revoked")
	}
	return nil
}

func TestRevocationDoesNotDependOnBackendAvailability(t *testing.T) {
	s, b, _ := newEditorFixture(t)
	tasks := &revokedEditorTasks{}
	s.tasks = tasks
	ctx := context.Background()
	status, err := s.Open(ctx, "owner", editorTestScope)
	if err != nil {
		t.Fatal(err)
	}
	_, lease, err := s.Access(ctx, "owner", editorTestScope, status.Generation)
	if err != nil {
		t.Fatal(err)
	}
	tasks.denied.Store(true)
	b.inspectError = errors.New("Docker unavailable")
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); s.Run(runCtx) }()
	defer func() { cancel(); <-done }()
	select {
	case <-lease.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("revoked connection retained while backend unavailable")
	}
}

func TestWorkspaceRemovalWaitRespectsRequestCancellation(t *testing.T) {
	s, _, _ := newEditorFixture(t)
	unlock, err := s.BeforeImportedTaskRemoval(context.Background(), "workspace", "github", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := s.Open(ctx, "owner", editorTestScope); result <- err }()
	select {
	case err := <-result:
		unlock()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("canceled wait: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		unlock()
		receivedErr := <-result
		t.Fatalf("workspace lock ignored request cancellation: %v", receivedErr)
	}
}

func TestFailedStopIsRetriedWithoutReopening(t *testing.T) {
	s, b, _ := newEditorFixture(t)
	ctx := context.Background()
	if _, err := s.Open(ctx, "owner", editorTestScope); err != nil {
		t.Fatal(err)
	}
	b.stopError = errors.New("daemon unavailable")
	if err := s.Stop(ctx, "owner", editorTestScope); err == nil {
		t.Fatal("expected unconfirmed stop")
	}
	if _, err := s.Open(ctx, "owner", editorTestScope); err == nil {
		t.Fatal("reopened before cleanup confirmed")
	}
	b.stopError = nil
	if err := s.reconcile(ctx, editorTestScope); err != nil {
		t.Fatal(err)
	}
	if b.found || b.starts != 1 {
		t.Fatalf("cleanup reopened editor: %+v", b)
	}
	status, err := s.Inspect(ctx, "owner", editorTestScope)
	if err != nil || status.State != "stopped" {
		t.Fatalf("cleanup status: %+v %v", status, err)
	}
	if _, err := s.Open(ctx, "new-owner", editorTestScope); err != nil {
		t.Fatalf("completed stop retained ownership: %v", err)
	}
}

func TestWorkspaceRemovalDoesNotBlockAnotherWorkspace(t *testing.T) {
	s, _, _ := newEditorFixture(t)
	ctx := context.Background()
	unlock, err := s.BeforeImportedTaskRemoval(ctx, "workspace", "github", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	otherCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	release, err := s.BeforeExecution(otherCtx, "org", "other-workspace", "other-task")
	if err != nil {
		t.Fatalf("unrelated workspace blocked: %v", err)
	}
	release()
}

func TestInspectionDoesNotRetainInactiveTaskLocks(t *testing.T) {
	s, _, _ := newEditorFixture(t)
	for i := range 200 {
		scope := Scope{"org", "workspace", fmt.Sprint(i)}
		if _, err := s.Inspect(context.Background(), "user", scope); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.entries) != 0 {
		t.Fatalf("retained %d inactive task locks", len(s.entries))
	}
}

func TestAuthorizationRevokesLeaseWhileLifecycleLockIsHeld(t *testing.T) {
	s, _, _ := newEditorFixture(t)
	tasks := &revokedEditorTasks{}
	s.tasks = tasks
	status, err := s.Open(context.Background(), "owner", editorTestScope)
	if err != nil {
		t.Fatal(err)
	}
	_, lease, err := s.Access(context.Background(), "owner", editorTestScope, status.Generation)
	if err != nil {
		t.Fatal(err)
	}
	// A stuck Docker/filesystem operation holds this same lifecycle lock.
	_, unlock, err := s.lockEntry(context.Background(), editorTestScope)
	if err != nil {
		t.Fatal(err)
	}
	tasks.denied.Store(true)
	select {
	case <-lease.Done():
	case <-time.After(2 * time.Second):
		unlock()
		t.Fatal("revocation waited for the lifecycle lock")
	}
	unlock()
	// Restoring authority must never reactivate the old generation's tickets.
	tasks.denied.Store(false)
	reopened, err := s.Open(context.Background(), "owner", editorTestScope)
	if err != nil || reopened.Generation == status.Generation {
		t.Fatalf("revoked generation reused: %+v %v", reopened, err)
	}
	if _, _, err := s.Access(context.Background(), "owner", editorTestScope, status.Generation); !errors.Is(err, ErrGeneration) {
		t.Fatalf("old ticket accepted: %v", err)
	}
}

func TestAuthorizationAdmissionTimeoutFailsClosed(t *testing.T) {
	s, _, _ := newEditorFixture(t)
	status, err := s.Open(context.Background(), "owner", editorTestScope)
	if err != nil {
		t.Fatal(err)
	}
	_, lease, err := s.Access(context.Background(), "owner", editorTestScope, status.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.authorizations.Acquire(context.Background(), 8); err != nil {
		t.Fatal(err)
	}
	defer s.authorizations.Release(8)
	select {
	case <-lease.Done():
	case <-time.After(4 * time.Second):
		t.Fatal("saturated authorization admission retained editor access")
	}
}

func TestCloseRevokesEditorLeases(t *testing.T) {
	s, _, _ := newEditorFixture(t)
	status, err := s.Open(context.Background(), "owner", editorTestScope)
	if err != nil {
		t.Fatal(err)
	}
	_, lease, err := s.Access(context.Background(), "owner", editorTestScope, status.Generation)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if lease.Err() == nil {
		t.Fatal("shutdown retained editor lease")
	}
}

type stalledLifecycleBackend struct {
	Backend
	started chan Scope
}

func (b *stalledLifecycleBackend) InspectEditor(ctx context.Context, scope Scope) (Instance, bool, error) {
	b.started <- scope
	<-ctx.Done()
	return Instance{}, false, ctx.Err()
}

func TestReconciliationBoundsConcurrentExecutorIOAndCancels(t *testing.T) {
	s, _, _ := newEditorFixture(t)
	backend := &stalledLifecycleBackend{started: make(chan Scope, 6)}
	s.backend = backend
	scopes := make([]Scope, 6)
	for i := range scopes {
		scopes[i] = Scope{"org", fmt.Sprint(i), "task"}
		e := s.entry(scopes[i])
		e.requested, e.user = true, "owner"
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.reconcileBatch(ctx, scopes) }()
	defer func() { cancel(); <-done }()
	// Independent workspaces progress concurrently, even while earlier I/O stalls.
	for range 4 {
		select {
		case <-backend.started:
		case <-time.After(time.Second):
			t.Fatal("stalled executor serialized unrelated workspaces")
		}
	}
	select {
	case <-backend.started:
		t.Fatal("unbounded executor I/O")
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconciliation did not stop on cancellation")
	}
}
