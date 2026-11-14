package taskeditor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
)

var (
	ErrUnavailable = errors.New("task editor unavailable")
	ErrOwner       = errors.New("task editor belongs to another user")
	ErrGeneration  = errors.New("task editor generation expired")
)

type (
	Scope struct{ OrganizationID, WorkspaceID, TaskID string }
	Mode  string
)

const (
	ReadOnly  Mode = "ro"
	ReadWrite Mode = "rw"
)

type Instance struct {
	GatewayToken                                  string `json:"-"`
	Scope                                         Scope
	ID, UserID, Generation, Fingerprint, Endpoint string
	Mode                                          Mode
}
type (
	Spec struct {
		Instance
		Discovery Discovery
	}
	Backend interface {
		StartEditor(context.Context, Spec) (Instance, error)
		InspectEditor(context.Context, Scope) (Instance, bool, error)
		StopEditor(context.Context, Instance) error
		ListEditors(context.Context) ([]Instance, error)
	}
)

// IdleReader must establish absence of executable worker ownership, not just an
// idle runtime status. Errors and uncertainty always deny writable access.
type (
	IdleReader interface {
		EditorIdle(context.Context, string, string, string) (bool, error)
	}
	Status struct {
		State      string            `json:"state"`
		Mode       Mode              `json:"mode,omitempty"`
		Generation string            `json:"generation,omitempty"`
		Missing    []MissingCheckout `json:"missing,omitempty"`
	}
)

type entry struct {
	refs          int // protected by TaskEditorService.mu
	lock          chan struct{}
	instance      Instance
	cancel        context.CancelFunc
	access        context.Context
	requested     bool
	stopRequested bool
	failed        bool
	user          string
}
type TaskEditorService struct {
	gates     map[string]*semaphore.Weighted
	tasks     TaskReader
	inspector CheckoutInspector
	backend   Backend
	idle      IdleReader
	mu        sync.Mutex
	entries   map[Scope]*entry
}

func NewTaskEditorService(tasks TaskReader, inspector CheckoutInspector, backend Backend, idle IdleReader) (*TaskEditorService, error) {
	if tasks == nil || inspector == nil || backend == nil || idle == nil {
		return nil, errors.New("task editor dependencies required")
	}
	return &TaskEditorService{tasks: tasks, inspector: inspector, backend: backend, idle: idle, entries: make(map[Scope]*entry), gates: make(map[string]*semaphore.Weighted)}, nil
}

func (s *TaskEditorService) entry(scope Scope) *entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[scope]
	if e == nil {
		e = &entry{lock: make(chan struct{}, 1)}
		s.entries[scope] = e
	}
	return e
}

func acquire(ctx context.Context, e *entry) error {
	select {
	case e.lock <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func release(e *entry) { <-e.lock }
func (s *TaskEditorService) authorize(ctx context.Context, user string, scope Scope) error {
	return s.tasks.AuthorizeExecution(ctx, user, scope.OrganizationID, scope.WorkspaceID, scope.TaskID, true)
}

func (s *TaskEditorService) Open(ctx context.Context, user string, scope Scope) (Status, error) {
	if err := s.authorize(ctx, user, scope); err != nil {
		return Status{}, err
	}
	e, unlock, err := s.lockEntry(ctx, scope)
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	status, err := s.open(ctx, e, user, scope)
	if !errors.Is(err, ErrOwner) {
		e.failed = e.stopRequested || (err != nil && !errors.Is(err, ErrUnavailable))
	}
	return status, err
}

func (s *TaskEditorService) open(ctx context.Context, e *entry, user string, scope Scope) (Status, error) {
	if err := checkOwner(e, user); err != nil {
		return Status{}, err
	}
	if e.stopRequested {
		return Status{}, ErrUnavailable
	}
	current, found, err := s.backend.InspectEditor(ctx, scope)
	if err != nil {
		if e.cancel != nil {
			e.cancel()
			e.cancel = nil
			e.access = nil
		}
		return Status{}, err
	}
	if found && current.UserID != user {
		return Status{}, ErrOwner
	}
	discovery, err := Discover(ctx, s.tasks, s.inspector, user, scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
	if err != nil {
		return Status{}, errors.Join(err, s.stop(ctx, e, current, found))
	}
	if len(discovery.Roots) == 0 {
		if err = s.stop(ctx, e, current, found); err != nil {
			return Status{}, err
		}
		return Status{State: "unavailable", Missing: discovery.Missing}, nil
	}
	mode := ReadOnly
	idle, idleErr := s.idle.EditorIdle(ctx, scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
	if idleErr == nil && idle {
		mode = ReadWrite
	}
	fingerprintBytes, _ := json.Marshal(discovery.Roots)
	// Sources are deliberately excluded from public JSON, but included in compatibility.
	for _, root := range discovery.Roots {
		fingerprintBytes = append(fingerprintBytes, []byte(root.Source)...)
	}
	fingerprint := sha256.Sum256(fingerprintBytes)
	compatible := found && current.Endpoint != "" && current.Mode == mode && current.Fingerprint == hex.EncodeToString(fingerprint[:])
	if !compatible {
		if err = s.stop(ctx, e, current, found); err != nil {
			return Status{}, err
		}
		// All execution deliveries take this same lock. Recheck immediately before creation.
		if mode == ReadWrite {
			idle, err = s.idle.EditorIdle(ctx, scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
			if err != nil || !idle {
				mode = ReadOnly
			}
		}
		e.user = user
		e.requested = true
		current, err = s.backend.StartEditor(ctx, Spec{Instance: Instance{Scope: scope, UserID: user, Generation: hex.EncodeToString(randomBytes()), Fingerprint: hex.EncodeToString(fingerprint[:]), Mode: mode}, Discovery: discovery})
		if err != nil {
			if current.ID != "" {
				err = errors.Join(err, s.stop(ctx, e, current, true))
			}
			return Status{}, err
		}
	}
	if e.instance.Generation != current.Generation || e.access == nil {
		if e.cancel != nil {
			e.cancel()
		}
		e.access, e.cancel = context.WithCancel(context.Background())
	}
	e.instance = current
	e.requested = true
	e.user = user
	return Status{State: "ready", Mode: current.Mode, Generation: current.Generation, Missing: discovery.Missing}, nil
}
func randomBytes() []byte { b := make([]byte, 32); _, _ = rand.Read(b); return b }
func revoke(e *entry) {
	if e.cancel != nil {
		e.cancel()
	}
	e.cancel = nil
	e.access = nil
}

func checkOwner(e *entry, user string) error {
	if (e.requested || e.stopRequested) && e.user != "" && e.user != user {
		return ErrOwner
	}
	return nil
}

func (s *TaskEditorService) stop(ctx context.Context, e *entry, current Instance, found bool) error {
	revoke(e)
	if !found && e.instance.ID != "" {
		current = e.instance
		found = true
	}
	if found {
		e.instance = current
		if err := s.backend.StopEditor(ctx, current); err != nil {
			e.stopRequested = true
			e.requested = false
			e.failed = true
			return err
		}
	}
	e.instance = Instance{}
	return nil
}

// finishStop retains cleanup intent until resource termination is confirmed.
func (s *TaskEditorService) finishStop(ctx context.Context, e *entry, scope Scope) error {
	revoke(e)
	current, found, err := s.backend.InspectEditor(ctx, scope)
	if err == nil {
		err = s.stop(ctx, e, current, found)
	} else if e.instance.ID != "" {
		err = s.stop(ctx, e, e.instance, true)
	}
	e.failed = err != nil
	if err == nil {
		e.stopRequested = false
		e.user = ""
	}
	return err
}

func (s *TaskEditorService) Inspect(ctx context.Context, user string, scope Scope) (Status, error) {
	if err := s.authorize(ctx, user, scope); err != nil {
		return Status{}, err
	}
	e, unlock, err := s.lockEntry(ctx, scope)
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	if err := checkOwner(e, user); err != nil {
		return Status{}, err
	}
	if e.failed {
		return Status{State: "failed"}, nil
	}
	current, found, err := s.backend.InspectEditor(ctx, scope)
	if err != nil {
		return Status{}, err
	}
	if !found {
		return Status{State: "stopped"}, nil
	}
	if current.UserID != user {
		return Status{}, ErrOwner
	}
	if current.Endpoint == "" {
		return Status{State: "failed"}, nil
	}
	return Status{State: "ready", Mode: current.Mode, Generation: current.Generation}, nil
}

func (s *TaskEditorService) Stop(ctx context.Context, user string, scope Scope) error {
	if err := s.authorize(ctx, user, scope); err != nil {
		return err
	}
	e, unlock, err := s.lockEntry(ctx, scope)
	if err != nil {
		return err
	}
	defer unlock()
	if err = checkOwner(e, user); err != nil {
		return err
	}
	// Known owners can revoke access immediately even if inspection is unavailable.
	if e.user == user {
		e.requested = false
		e.stopRequested = true
		revoke(e)
	}
	current, found, err := s.backend.InspectEditor(ctx, scope)
	if err != nil {
		e.failed = true
		return err
	}
	if found && current.UserID != user {
		return ErrOwner
	}
	e.user = user
	e.requested = false
	e.stopRequested = true
	err = s.stop(ctx, e, current, found)
	e.failed = err != nil
	if err == nil {
		e.stopRequested = false
		e.user = ""
	}
	return err
}

// BeforeExecution holds the task lock through work delivery. Already accepted
// worker commands are covered by denying write access while any owner survives.
func (s *TaskEditorService) BeforeExecution(ctx context.Context, org, workspace, task string) (func(), error) {
	return s.beforeTransition(ctx, Scope{org, workspace, task}, false)
}

// Access binds each proxy request to a currently authorized user and generation.
func (s *TaskEditorService) Access(ctx context.Context, user string, scope Scope, generation string) (Instance, context.Context, error) {
	if err := s.authorize(ctx, user, scope); err != nil {
		return Instance{}, nil, err
	}
	e, unlock, err := s.lockEntry(ctx, scope)
	if err != nil {
		return Instance{}, nil, err
	}
	defer unlock()
	if e.instance.Generation != generation || e.instance.UserID != user || e.access == nil {
		return Instance{}, nil, ErrGeneration
	}
	return e.instance, e.access, nil
}

// ReconcileStartup runs before any recovery/delivery. Stale editor processes are
// stopped by concrete ID; saved repository files and agent workers are untouched.
func (s *TaskEditorService) ReconcileStartup(ctx context.Context) error {
	instances, err := s.backend.ListEditors(ctx)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		if err = s.backend.StopEditor(ctx, instance); err != nil {
			return err
		}
	}
	return nil
}

// Run refreshes only explicitly opened editors. Revoked authority tears down an
// editor; a missing task is therefore cleaned up even after imported-task removal.
func (s *TaskEditorService) reconcile(ctx context.Context, scope Scope) error {
	e, unlock, err := s.lockEntry(ctx, scope)
	if err != nil {
		return err
	}
	defer unlock()
	if e.stopRequested {
		return s.finishStop(ctx, e, scope)
	}
	if !e.requested {
		return nil
	}
	if err = s.authorize(ctx, e.user, scope); err != nil {
		revoke(e)
		e.requested = false
		e.stopRequested = true
		return errors.Join(err, s.finishStop(ctx, e, scope))
	}
	_, err = s.open(ctx, e, e.user, scope)
	e.failed = err != nil
	return err
}

func (s *TaskEditorService) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		s.mu.Lock()
		scopes := make([]Scope, 0, len(s.entries))
		for scope := range s.entries {
			scopes = append(scopes, scope)
		}
		s.mu.Unlock()
		for _, scope := range scopes {
			opCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			_ = s.reconcile(opCtx, scope)
			cancel()
			if ctx.Err() != nil {
				return
			}
		}
	}
}

// BeforeTaskMutation prevents a saved selection from racing mounted checkouts.
func (s *TaskEditorService) BeforeTaskMutation(ctx context.Context, org, workspace, task string) (func(), error) {
	return s.beforeTransition(ctx, Scope{org, workspace, task}, true)
}

func (s *TaskEditorService) beforeTransition(ctx context.Context, scope Scope, stopReadOnly bool) (func(), error) {
	e, unlock, err := s.lockEntry(ctx, scope)
	if err != nil {
		return nil, err
	}
	current, found, err := s.backend.InspectEditor(ctx, scope)
	if err == nil && found && (current.Mode == ReadWrite || stopReadOnly) {
		err = s.stop(ctx, e, current, true)
	}
	if err != nil {
		revoke(e)
		unlock()
		return nil, err
	}
	return unlock, nil
}

// BeforeImportedTaskRemoval fences bulk source deletion without discovering task
// IDs through new persistence queries. Retained/manual tasks stay connected.
// Another source of the same provider may reconnect when its ID is absent here.
func (s *TaskEditorService) BeforeImportedTaskRemoval(ctx context.Context, workspace, provider string, seen []string) (func(), error) {
	unlock, err := s.lockWorkspace(ctx, workspace, true)
	if err != nil {
		return nil, err
	}
	instances, err := s.backend.ListEditors(ctx)
	if err != nil {
		unlock()
		return nil, err
	}
	retainedIDs := make(map[string]struct{}, len(seen))
	for _, id := range seen {
		retainedIDs[id] = struct{}{}
	}
	for _, instance := range instances {
		if instance.Scope.WorkspaceID != workspace {
			continue
		}
		task, taskErr := s.tasks.Get(ctx, instance.UserID, instance.Scope.OrganizationID, workspace, instance.Scope.TaskID)
		if taskErr == nil {
			if task.SourceProvider != provider {
				continue
			}
			_, retained := retainedIDs[task.SourceID]
			if retained {
				continue
			}
		}
		e := s.entry(instance.Scope)
		if err = s.stop(ctx, e, instance, true); err != nil {
			unlock()
			return nil, err
		}
	}
	return unlock, nil
}

const workspaceGateCapacity int64 = 1 << 30

func (s *TaskEditorService) lockWorkspace(ctx context.Context, workspace string, exclusive bool) (func(), error) {
	s.mu.Lock()
	gate := s.gates[workspace]
	if gate == nil {
		gate = semaphore.NewWeighted(workspaceGateCapacity)
		s.gates[workspace] = gate
	}
	s.mu.Unlock()
	weight := int64(1)
	if exclusive {
		weight = workspaceGateCapacity
	}
	if err := gate.Acquire(ctx, weight); err != nil {
		return nil, err
	}
	return func() { gate.Release(weight) }, nil
}

// borrowEntry counts holders and waiters before exposing a lock. This avoids
// deleting a lock while another caller is waiting on the same task.
func (s *TaskEditorService) borrowEntry(scope Scope) *entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[scope]
	if e == nil {
		e = &entry{lock: make(chan struct{}, 1)}
		s.entries[scope] = e
	}
	e.refs++
	return e
}

func (s *TaskEditorService) returnEntry(scope Scope, e *entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.refs--
	// With no holders/waiters, and while workspace admission is held, no other
	// lifecycle operation can mutate this entry. Keep failed/pending state visible.
	if e.refs == 0 && !e.requested && !e.stopRequested && !e.failed && e.instance.ID == "" && e.access == nil {
		delete(s.entries, scope)
	}
}

func (s *TaskEditorService) lockEntry(ctx context.Context, scope Scope) (*entry, func(), error) {
	unlockWorkspace, err := s.lockWorkspace(ctx, scope.WorkspaceID, false)
	if err != nil {
		return nil, nil, err
	}
	e := s.borrowEntry(scope)
	if err = acquire(ctx, e); err != nil {
		s.returnEntry(scope, e)
		unlockWorkspace()
		return nil, nil, err
	}
	return e, func() { s.returnEntry(scope, e); release(e); unlockWorkspace() }, nil
}
