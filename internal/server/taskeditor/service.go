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
	lock      chan struct{}
	instance  Instance
	cancel    context.CancelFunc
	access    context.Context
	requested bool
	failed    bool
	user      string
}
type TaskEditorService struct {
	barrier   sync.RWMutex
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
	return &TaskEditorService{tasks: tasks, inspector: inspector, backend: backend, idle: idle, entries: make(map[Scope]*entry)}, nil
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
	s.barrier.RLock()
	defer s.barrier.RUnlock()
	if err := s.authorize(ctx, user, scope); err != nil {
		return Status{}, err
	}
	e := s.entry(scope)
	if err := acquire(ctx, e); err != nil {
		return Status{}, err
	}
	defer release(e)
	status, err := s.open(ctx, e, user, scope)
	if !errors.Is(err, ErrOwner) {
		e.failed = err != nil
	}
	return status, err
}

func (s *TaskEditorService) open(ctx context.Context, e *entry, user string, scope Scope) (Status, error) {
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
		_ = s.stop(ctx, e, current, found)
		return Status{}, err
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
func (s *TaskEditorService) stop(ctx context.Context, e *entry, current Instance, found bool) error {
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
		e.access = nil
	}
	if found {
		if err := s.backend.StopEditor(ctx, current); err != nil {
			return err
		}
	}
	e.instance = Instance{}
	return nil
}

func (s *TaskEditorService) Inspect(ctx context.Context, user string, scope Scope) (Status, error) {
	s.barrier.RLock()
	defer s.barrier.RUnlock()
	if err := s.authorize(ctx, user, scope); err != nil {
		return Status{}, err
	}
	e := s.entry(scope)
	if err := acquire(ctx, e); err != nil {
		return Status{}, err
	}
	defer release(e)
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
	return Status{State: "ready", Mode: current.Mode, Generation: current.Generation}, nil
}

func (s *TaskEditorService) Stop(ctx context.Context, user string, scope Scope) error {
	s.barrier.RLock()
	defer s.barrier.RUnlock()
	if err := s.authorize(ctx, user, scope); err != nil {
		return err
	}
	e := s.entry(scope)
	if err := acquire(ctx, e); err != nil {
		return err
	}
	defer release(e)
	current, found, err := s.backend.InspectEditor(ctx, scope)
	if err != nil {
		return err
	}
	if found && current.UserID != user {
		return ErrOwner
	}
	e.requested = false
	return s.stop(ctx, e, current, found)
}

// BeforeExecution holds the task lock through work delivery. Already accepted
// worker commands are covered by denying write access while any owner survives.
func (s *TaskEditorService) BeforeExecution(ctx context.Context, org, workspace, task string) (func(), error) {
	s.barrier.RLock()
	e := s.entry(Scope{org, workspace, task})
	if err := acquire(ctx, e); err != nil {
		s.barrier.RUnlock()
		return nil, err
	}
	current, found, err := s.backend.InspectEditor(ctx, Scope{org, workspace, task})
	if err == nil && found && current.Mode == ReadWrite {
		err = s.stop(ctx, e, current, true)
	}
	if err != nil {
		release(e)
		s.barrier.RUnlock()
		return nil, err
	}
	return func() { release(e); s.barrier.RUnlock() }, nil
}

// Access binds each proxy request to a currently authorized user and generation.
func (s *TaskEditorService) Access(ctx context.Context, user string, scope Scope, generation string) (Instance, context.Context, error) {
	s.barrier.RLock()
	defer s.barrier.RUnlock()
	if err := s.authorize(ctx, user, scope); err != nil {
		return Instance{}, nil, err
	}
	e := s.entry(scope)
	if err := acquire(ctx, e); err != nil {
		return Instance{}, nil, err
	}
	defer release(e)
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
			s.barrier.RLock()
			e := s.entry(scope)
			opCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			if acquire(opCtx, e) == nil {
				if e.requested {
					user := e.user
					// Handoff clears the concrete instance but keeps the requesting user.
					if user != "" {
						if err := s.authorize(opCtx, user, scope); err != nil {
							current, found, inspectErr := s.backend.InspectEditor(opCtx, scope)
							if inspectErr == nil {
								if stopErr := s.stop(opCtx, e, current, found); stopErr == nil {
									e.requested = false
								} else {
									e.failed = true
								}
							}
						} else {
							_, openErr := s.open(opCtx, e, user, scope)
							e.failed = openErr != nil
						}
					}
				}
				release(e)
			}
			cancel()
			s.barrier.RUnlock()
		}
	}
}

// BeforeTaskMutation prevents a saved selection from racing mounted checkouts.
func (s *TaskEditorService) BeforeTaskMutation(ctx context.Context, org, workspace, task string) (func(), error) {
	s.barrier.RLock()
	e := s.entry(Scope{org, workspace, task})
	if err := acquire(ctx, e); err != nil {
		s.barrier.RUnlock()
		return nil, err
	}
	current, found, err := s.backend.InspectEditor(ctx, Scope{org, workspace, task})
	if err == nil {
		err = s.stop(ctx, e, current, found)
	}
	if err != nil {
		release(e)
		s.barrier.RUnlock()
		return nil, err
	}
	return func() { release(e); s.barrier.RUnlock() }, nil
}

// BeforeImportedTaskRemoval fences bulk source deletion without discovering task
// IDs through new persistence queries. Retained/manual tasks stay connected.
// Another source of the same provider may reconnect when its ID is absent here.
func (s *TaskEditorService) BeforeImportedTaskRemoval(ctx context.Context, workspace, provider string, seen []string) (func(), error) {
	s.barrier.Lock()
	instances, err := s.backend.ListEditors(ctx)
	if err != nil {
		s.barrier.Unlock()
		return nil, err
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
			retained := false
			for _, sourceID := range seen {
				if task.SourceID == sourceID {
					retained = true
					break
				}
			}
			if retained {
				continue
			}
		}
		e := s.entry(instance.Scope)
		if err = s.stop(ctx, e, instance, true); err != nil {
			s.barrier.Unlock()
			return nil, err
		}
	}
	return s.barrier.Unlock, nil
}
