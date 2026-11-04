package postgres

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
)

type deliveryTestDispatcher struct {
	db                      *PostgresDatabase
	launches                *AgentLaunchPostgresRepository
	orgID                   string
	calls                   atomic.Int32
	failure                 error
	failureAfterReservation error
	acceptanceError         error
	reserved                chan struct{}
	release                 chan struct{}
	cancel                  context.CancelFunc
}

func (d *deliveryTestDispatcher) TaskCommandAccepted(ctx context.Context, orgID, workspaceID, taskID, commandID, _ string) (bool, error) {
	if d.acceptanceError != nil {
		return false, d.acceptanceError
	}
	return d.launches.TaskCommand(ctx, orgID, workspaceID, taskID, commandID, "")
}

func (d *deliveryTestDispatcher) StartTask(ctx context.Context, _, _ string, command task.StartCommand) error {
	d.calls.Add(1)
	// External dispatch must hold neither a pool connection nor a transaction.
	if d.db.pool.Stat().AcquiredConns() != 0 {
		return errors.New("database connection held across external dispatch")
	}
	if d.failure != nil {
		return d.failure
	}
	runID, err := ids.NewUUID()
	if err != nil {
		return err
	}
	sessionID, err := ids.NewUUID()
	if err != nil {
		return err
	}
	config := orchestration.RunConfig{RuntimeConfig: orchestration.RuntimeConfig{OrganizationID: d.orgID, WorkspaceID: command.Task.WorkspaceID, TaskID: command.Task.ID, TaskSessionID: sessionID, RunID: runID, Runtime: "opencode", Agent: orchestration.AgentDefinition{Name: "fixture", Instructions: "fixture"}, Model: orchestration.ModelSelection{Provider: "test", ID: "model"}}, InitialCommandID: command.CommandID, InitialPrompt: "fixture prompt", DeliveryToken: command.DeliveryToken}
	if err = d.launches.ReserveSessionRun(ctx, config, command.Task.StageVersion); err != nil {
		return err
	}
	if d.reserved != nil {
		close(d.reserved)
	}
	if d.release != nil {
		select {
		case <-d.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if d.cancel != nil {
		d.cancel()
	}
	return d.failureAfterReservation
}

func testWorkflowDeliveryClaims(t *testing.T, db *PostgresDatabase, orgID, workspaceID, actorID, agentID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	q := sqlc.New(db.pool)
	tasks := NewTaskPostgresRepository(db)
	coordinator := NewExecutionCommandPostgresRepository(db)
	launches := NewAgentLaunchPostgresRepository(db, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	taskService := task.NewTaskService(tasks, NewTaskCreationPostgresRepository(db), NewTaskUpdatePostgresRepository(db), newTaskWorkflowService(tasks), workflowTestAccess{})
	newID := func() string {
		id, err := ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	enqueue := func() workflowautomation.ExecutionCommand {
		value, err := taskService.Create(ctx, actorID, orgID, workspaceID, task.TaskInput{WorkflowID: workspaceID, Title: "Delivery claim", AssigneeAgentID: agentID})
		if err != nil {
			t.Fatal(err)
		}
		command := workflowautomation.ExecutionCommand{ID: newID(), OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: value.ID, ActorID: actorID, StageVersion: value.StageVersion}
		if err = q.EnqueueWorkflowCommand(ctx, sqlc.EnqueueWorkflowCommandParams{ID: command.ID, WorkspaceID: workspaceID, TaskID: value.ID, ActorID: actorID, StageVersion: value.StageVersion, Options: []byte(`{"action":"start","sessionMode":"new"}`)}); err != nil {
			t.Fatal(err)
		}
		if err = q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: workspaceID, ID: value.ID, StageVersion: value.StageVersion, StageWorkState: "queued"}); err != nil {
			t.Fatal(err)
		}
		return command
	}
	expire := func(id string) {
		if _, err := db.pool.Exec(ctx, "UPDATE workflow_commands SET claim_until=now()-interval '1 second' WHERE id=$1", id); err != nil {
			t.Fatal(err)
		}
	}
	claim := func() workflowautomation.ExecutionClaim {
		got, found, err := coordinator.ClaimExecutionCommand(ctx, newID(), time.Minute)
		if err != nil || !found {
			t.Fatalf("claim: found=%v error=%v", found, err)
		}
		return got
	}
	state := func(command workflowautomation.ExecutionCommand, wantState, wantWork, reason string) {
		var actualState, work, diagnostic string
		if err := db.pool.QueryRow(ctx, "SELECT c.state,t.stage_work_state,c.delivery_error FROM workflow_commands c JOIN tasks t ON t.id=c.task_id WHERE c.id=$1", command.ID).Scan(&actualState, &work, &diagnostic); err != nil {
			t.Fatal(err)
		}
		if actualState != wantState || work != wantWork || diagnostic != reason {
			t.Fatalf("state=%s work=%s diagnostic=%s", actualState, work, diagnostic)
		}
	}
	service := func(dispatcher *deliveryTestDispatcher) *workflowautomation.WorkflowExecutionService {
		return workflowautomation.NewWorkflowExecutionService(coordinator, taskService, dispatcher, logger)
	}
	t.Run("two workers claim once and stale owners cannot reserve or acknowledge", func(t *testing.T) {
		command := enqueue()
		results := make(chan workflowautomation.ExecutionClaim, 2)
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() {
				got, found, err := coordinator.ClaimExecutionCommand(ctx, newID(), time.Minute)
				if err != nil {
					errs <- err
				}
				if found {
					results <- got
				}
			})
		}
		wg.Wait()
		close(results)
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		var claims []workflowautomation.ExecutionClaim
		for got := range results {
			claims = append(claims, got)
		}
		if len(claims) != 1 || claims[0].Command.ID != command.ID {
			t.Fatalf("claims=%+v", claims)
		}
		first := claims[0]
		expire(command.ID)
		second := claim()
		called := false
		err := coordinator.CompleteExecutionCommand(ctx, first, func(bool) workflowautomation.ExecutionDeliveryEffects {
			called = true
			return workflowautomation.ExecutionDeliveryEffects{CommandState: "delivered", WorkState: task.WorkStateRunning}
		})
		if !errors.Is(err, workflowautomation.ErrStaleDeliveryClaim) || called {
			t.Fatalf("stale ack: %v callback=%v", err, called)
		}
		value, err := tasks.Task(ctx, orgID, workspaceID, command.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		dispatcher := &deliveryTestDispatcher{db: db, launches: launches, orgID: orgID}
		err = dispatcher.StartTask(ctx, actorID, orgID, task.StartCommand{Task: value, CommandID: command.ID, DeliveryToken: first.Token})
		if !errors.Is(err, modelerrors.ErrConflict) {
			t.Fatalf("stale claimant reserved: %v", err)
		}
		if err = coordinator.CompleteExecutionCommand(ctx, second, func(bool) workflowautomation.ExecutionDeliveryEffects {
			return workflowautomation.ExecutionDeliveryEffects{CommandState: "failed", WorkState: task.WorkStateFailed, FailureReason: "delivery_uncertain"}
		}); err != nil {
			t.Fatal(err)
		}
		state(command, "failed", "failed", "delivery_uncertain")
	})
	t.Run("crash after claim does not automatically start", func(t *testing.T) {
		command := enqueue()
		_ = claim()
		expire(command.ID)
		dispatcher := &deliveryTestDispatcher{db: db, launches: launches, orgID: orgID}
		if _, err := service(dispatcher).ProcessNextCommand(ctx); err != nil {
			t.Fatal(err)
		}
		if dispatcher.calls.Load() != 0 {
			t.Fatal("expired claim launched work")
		}
		state(command, "failed", "failed", "delivery_uncertain")
	})
	t.Run("accepted reservation survives loss before acknowledgement", func(t *testing.T) {
		command := enqueue()
		first := claim()
		value, err := tasks.Task(ctx, orgID, workspaceID, command.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		dispatcher := &deliveryTestDispatcher{db: db, launches: launches, orgID: orgID}
		if err = dispatcher.StartTask(ctx, actorID, orgID, task.StartCommand{Task: value, CommandID: command.ID, DeliveryToken: first.Token}); err != nil {
			t.Fatal(err)
		}
		expire(command.ID)
		if _, err = service(dispatcher).ProcessNextCommand(ctx); err != nil {
			t.Fatal(err)
		}
		if dispatcher.calls.Load() != 1 {
			t.Fatal("accepted run was launched twice")
		}
		state(command, "delivered", "running", "")
	})
	t.Run("ordinary launch failure never retries", func(t *testing.T) {
		command := enqueue()
		dispatcher := &deliveryTestDispatcher{db: db, launches: launches, orgID: orgID, failure: errors.New("ordinary failure")}
		execution := service(dispatcher)
		if _, err := execution.ProcessNextCommand(ctx); err != nil {
			t.Fatal(err)
		}
		if found, err := execution.ProcessNextCommand(ctx); err != nil || found {
			t.Fatalf("failed command retried: %v %v", found, err)
		}
		if dispatcher.calls.Load() != 1 {
			t.Fatal("ordinary failure retried")
		}
		state(command, "failed", "failed", "")
	})
	t.Run("unavailable agentctl retains the reserved owner", func(t *testing.T) {
		command := enqueue()
		dispatcher := &deliveryTestDispatcher{db: db, launches: launches, orgID: orgID, failureAfterReservation: errors.New("agentctl unavailable")}
		execution := service(dispatcher)
		if _, err := execution.ProcessNextCommand(ctx); err != nil {
			t.Fatal(err)
		}
		if found, err := execution.ProcessNextCommand(ctx); found || err != nil {
			t.Fatalf("unavailable worker command repeated: %v %v", found, err)
		}
		runs, err := launches.ListTaskLaunches(ctx, orgID, workspaceID, command.TaskID)
		if err != nil || len(runs) != 1 || runs[0].Status != orchestration.LaunchStarting {
			t.Fatalf("uncertain owner released or duplicated: %+v %v", runs, err)
		}
		var accepted int
		if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM agent_prompt_outbox WHERE command_id=$1", command.ID).Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted != 1 || dispatcher.calls.Load() != 1 {
			t.Fatalf("duplicate accepted commands: %d", accepted)
		}
		state(command, "delivered", "running", "")
	})
	t.Run("acknowledgement failure reconciles reserved run", func(t *testing.T) {
		command := enqueue()
		dispatchCtx, stop := context.WithCancel(ctx)
		defer stop()
		dispatcher := &deliveryTestDispatcher{db: db, launches: launches, orgID: orgID, cancel: stop}
		if _, err := service(dispatcher).ProcessNextCommand(dispatchCtx); !errors.Is(err, context.Canceled) {
			t.Fatalf("ack failure missing: %v", err)
		}
		state(command, "pending", "queued", "")
		expire(command.ID)
		dispatcher.cancel = nil
		if _, err := service(dispatcher).ProcessNextCommand(ctx); err != nil {
			t.Fatal(err)
		}
		if dispatcher.calls.Load() != 1 {
			t.Fatal("ack failure duplicated execution")
		}
		state(command, "delivered", "running", "")
	})
	t.Run("cancellation during acceptance reconciles without replacement", func(t *testing.T) {
		command := enqueue()
		dispatchCtx, stop := context.WithCancel(ctx)
		dispatcher := &deliveryTestDispatcher{db: db, launches: launches, orgID: orgID, failure: context.Canceled}
		// Cancel during dispatch, after durable claim acceptance.
		dispatcher.cancel = stop
		// Cancellation after reservation leaves acceptance for reconciliation.
		dispatcher.failure = nil
		dispatcher.reserved = make(chan struct{})
		dispatcher.release = make(chan struct{})
		result := make(chan error, 1)
		go func() { _, err := service(dispatcher).ProcessNextCommand(dispatchCtx); result <- err }()
		select {
		case <-dispatcher.reserved:
			stop()
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation: %v", err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		expire(command.ID)
		if _, err := service(dispatcher).ProcessNextCommand(ctx); err != nil {
			t.Fatal(err)
		}
		if dispatcher.calls.Load() != 1 {
			t.Fatal("cancelled delivery was dispatched again")
		}
		state(command, "delivered", "running", "")
	})
	t.Run("unavailable acceptance store leaves uncertain command pending", func(t *testing.T) {
		command := enqueue()
		dispatcher := &deliveryTestDispatcher{db: db, launches: launches, orgID: orgID, acceptanceError: errors.New("unavailable")}
		if _, err := service(dispatcher).ProcessNextCommand(ctx); err == nil {
			t.Fatal("unavailable acceptance store acknowledged")
		}
		expire(command.ID)
		if _, err := service(dispatcher).ProcessNextCommand(ctx); err == nil {
			t.Fatal("uncertain acceptance repeated")
		}
		state(command, "pending", "queued", "")
		expire(command.ID)
		dispatcher.acceptanceError = nil
		if _, err := service(dispatcher).ProcessNextCommand(ctx); err != nil {
			t.Fatal(err)
		}
		if dispatcher.calls.Load() != 0 {
			t.Fatal("unavailable acceptance store started work")
		}
		state(command, "failed", "failed", "delivery_uncertain")
	})
	t.Run("overlapping workers share one reserved execution", func(t *testing.T) {
		command := enqueue()
		dispatcher := &deliveryTestDispatcher{db: db, launches: launches, orgID: orgID, reserved: make(chan struct{}), release: make(chan struct{})}
		done := make(chan error, 1)
		go func() { _, err := service(dispatcher).ProcessNextCommand(ctx); done <- err }()
		select {
		case <-dispatcher.reserved:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		expire(command.ID)
		if _, err := service(dispatcher).ProcessNextCommand(ctx); err != nil {
			t.Fatal(err)
		}
		close(dispatcher.release)
		select {
		case err := <-done:
			if !errors.Is(err, workflowautomation.ErrStaleDeliveryClaim) {
				t.Fatalf("old worker acknowledged successor: %v", err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		if dispatcher.calls.Load() != 1 {
			t.Fatal("overlap duplicated execution")
		}
		state(command, "delivered", "running", "")
	})
}
