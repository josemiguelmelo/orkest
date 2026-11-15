package taskeditor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"syscall"
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
	GatewayTokenRequired                          bool   `json:"-"`
	Scope                                         Scope
	ID, UserID, Generation, Fingerprint, Endpoint string
	Mode                                          Mode
}
type (
	Spec struct {
		Instance
		Discovery Discovery
	}
	// EditorExecutor couples storage inspection with the matching editor lifecycle.
	// Absence is distinct from uncertain observation; Stop must confirm termination
	// and reject stale identity. Implementations borrow their infrastructure clients.
	EditorExecutor interface {
		CheckoutInspector
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
	lastWarning   time.Time // protected by TaskEditorService.mu
	warningCode   string    // protected by TaskEditorService.mu
}

type lifecycleError struct {
	stage string
	err   error
}

func (e lifecycleError) Error() string { return e.err.Error() }
func (e lifecycleError) Unwrap() error { return e.err }

func atLifecycleStage(stage string, err error) error {
	if err == nil {
		return nil
	}
	return lifecycleError{stage: stage, err: err}
}

func lifecycleErrorStage(err error) string {
	var staged interface{ LifecycleStage() string }
	if errors.As(err, &staged) {
		return staged.LifecycleStage()
	}
	var lifecycleErr lifecycleError
	if errors.As(err, &lifecycleErr) {
		return lifecycleErr.stage
	}
	return "authorize_or_reconcile"
}

func lifecycleFailureResource(err error) string {
	var resource interface{ FailureResource() string }
	if errors.As(err, &resource) {
		return resource.FailureResource()
	}
	return ""
}

type TaskEditorService struct {
	lifetime       context.Context
	close          context.CancelFunc
	authorizations *semaphore.Weighted
	gates          map[string]*semaphore.Weighted
	tasks          TaskReader
	backend        EditorExecutor
	idle           IdleReader
	mu             sync.Mutex
	entries        map[Scope]*entry
}

func NewTaskEditorService(tasks TaskReader, backend EditorExecutor, idle IdleReader) (*TaskEditorService, error) {
	if tasks == nil || backend == nil || idle == nil {
		return nil, errors.New("task editor dependencies required")
	}
	lifetime, cancel := context.WithCancel(context.Background())
	return &TaskEditorService{lifetime: lifetime, close: cancel, authorizations: semaphore.NewWeighted(8), tasks: tasks, backend: backend, idle: idle, entries: make(map[Scope]*entry), gates: make(map[string]*semaphore.Weighted)}, nil
}

// Close revokes active leases and stops their authorization watchdogs.
func (s *TaskEditorService) Close() { s.close() }

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
	if err != nil && !errors.Is(err, ErrOwner) {
		slog.WarnContext(ctx, "task editor open failed",
			"organization_id", scope.OrganizationID,
			"workspace_id", scope.WorkspaceID,
			"task_id", scope.TaskID,
			"stage", lifecycleErrorStage(err),
			"failure", lifecycleFailureCode(err),
			"failure_resource", lifecycleFailureResource(err),
		)
	}
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
		return Status{}, atLifecycleStage("inspect", err)
	}
	if found && current.UserID != user {
		return Status{}, ErrOwner
	}
	discovery, err := Discover(ctx, s.tasks, s.backend, user, scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
	if err != nil {
		return Status{}, errors.Join(atLifecycleStage("discover_workspace", err), atLifecycleStage("stop_after_discovery_failure", s.stop(ctx, e, current, found)))
	}
	if len(discovery.Roots) == 0 {
		if err = s.stop(ctx, e, current, found); err != nil {
			return Status{}, atLifecycleStage("stop_missing_workspace", err)
		}
		return Status{State: "unavailable", Missing: discovery.Missing}, nil
	}
	mode := ReadOnly
	idle, idleErr := s.idle.EditorIdle(ctx, scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
	if idleErr == nil && idle {
		mode = ReadWrite
	}
	fingerprint := workspaceFingerprint(discovery, mode)
	compatible := found && current.Endpoint != "" && (!current.GatewayTokenRequired || current.GatewayToken != "") && current.Mode == mode && current.Fingerprint == fingerprint
	// A revoked lease cannot be renewed under the same generation: old browser
	// tickets must stay invalid even if authority is subsequently restored.
	if e.instance.Generation == current.Generation && (e.access == nil || e.access.Err() != nil) {
		compatible = false
	}
	if !compatible {
		if err = s.stop(ctx, e, current, found); err != nil {
			return Status{}, atLifecycleStage("stop_incompatible_editor", err)
		}
		// All execution deliveries take this same lock. Recheck immediately before creation.
		if mode == ReadWrite {
			idle, err = s.idle.EditorIdle(ctx, scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
			if err != nil || !idle {
				mode = ReadOnly
			}
		}
		fingerprint = workspaceFingerprint(discovery, mode)
		e.user = user
		e.requested = true
		current, err = s.backend.StartEditor(ctx, Spec{Instance: Instance{Scope: scope, UserID: user, Generation: hex.EncodeToString(randomBytes()), Fingerprint: fingerprint, Mode: mode}, Discovery: discovery})
		if err != nil {
			if current.ID != "" {
				err = errors.Join(err, s.stop(ctx, e, current, true))
			}
			return Status{}, atLifecycleStage("start", err)
		}
	}
	if e.instance.Generation != current.Generation || e.access == nil {
		if e.cancel != nil {
			e.cancel()
		}
		e.access, e.cancel = context.WithCancel(s.lifetime)
		go s.watchAuthorization(e.access, e.cancel, user, scope)
	}
	e.instance = current
	e.requested = true
	e.user = user
	return Status{State: "ready", Mode: current.Mode, Generation: current.Generation, Missing: discovery.Missing}, nil
}

// workspaceFingerprint includes private references without exposing them in the
// public manifest. Structured encoding prevents ambiguous concatenated references.
func workspaceFingerprint(discovery Discovery, mode Mode) string {
	type rootIdentity struct{ Name, Path, StorageRef string }
	roots := make([]rootIdentity, len(discovery.Roots))
	for index, root := range discovery.Roots {
		roots[index] = rootIdentity(root)
	}
	encoded, _ := json.Marshal(struct {
		Roots            []rootIdentity
		Mode             Mode
		CompatibilityKey string
	}{roots, mode, discovery.CompatibilityKey})
	fingerprint := sha256.Sum256(encoded)
	return hex.EncodeToString(fingerprint[:])
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
	if e.instance.Generation != generation || e.instance.UserID != user || e.access == nil || e.access.Err() != nil {
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
	defer s.Close()
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
		s.reconcileBatch(ctx, scopes)
	}
}

// This path never takes lifecycle/task/workspace locks or calls the executor or
// filesystem. Every lease independently fails closed if admission or authority
// cannot be established within the deadline, even when lifecycle workers stall.
func (s *TaskEditorService) watchAuthorization(ctx context.Context, revoke context.CancelFunc, user string, scope Scope) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := s.authorizations.Acquire(checkCtx, 1)
		if err == nil {
			err = s.authorize(checkCtx, user, scope)
			s.authorizations.Release(1)
		}
		err = errors.Join(err, checkCtx.Err())
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			revoke()
			slog.Default().Warn("task editor access revoked", "organization_id", scope.OrganizationID, "workspace_id", scope.WorkspaceID, "task_id", scope.TaskID, "reason", "authorization_unavailable")
			return
		}
	}
}

func (s *TaskEditorService) reconcileBatch(ctx context.Context, scopes []Scope) {
	work := make(chan Scope)
	var workers sync.WaitGroup
	for range min(4, len(scopes)) {
		workers.Go(func() {
			for scope := range work {
				opCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				if err := s.reconcile(opCtx, scope); err != nil && ctx.Err() == nil {
					failure := lifecycleFailureCode(err)
					if s.shouldLogReconcileFailure(scope, failure, time.Now()) {
						slog.Default().Warn("task editor reconciliation deferred", "organization_id", scope.OrganizationID, "workspace_id", scope.WorkspaceID, "task_id", scope.TaskID, "reason", "lifecycle_unavailable", "stage", lifecycleErrorStage(err), "failure", failure, "failure_resource", lifecycleFailureResource(err))
					}
				}
				cancel()
			}
		})
	}
	defer workers.Wait()
	defer close(work)
	for _, scope := range scopes {
		select {
		case work <- scope:
		case <-ctx.Done():
			return
		}
	}
}

func (s *TaskEditorService) shouldLogReconcileFailure(scope Scope, failure string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[scope]
	if e == nil {
		return true
	}
	if e.warningCode == failure && now.Sub(e.lastWarning) < 30*time.Second {
		return false
	}
	e.warningCode = failure
	e.lastWarning = now
	return true
}

func lifecycleFailureCode(err error) string {
	var coded interface{ FailureCode() string }
	if errors.As(err, &coded) {
		return coded.FailureCode()
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection_refused"
	}
	var sqlState interface{ SQLState() string }
	if errors.As(err, &sqlState) {
		return "postgres_" + sqlState.SQLState()
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "network_timeout"
	}
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			break
		}
		err = unwrapped
	}
	return fmt.Sprintf("%T", err)
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
