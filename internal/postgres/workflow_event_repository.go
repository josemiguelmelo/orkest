package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/workflow"
)

func enqueueTaskEvent(ctx context.Context, q *sqlc.Queries, workspaceID, taskID, actorID string, version int64, event workflow.EventType, cause string, correlation ...string) error {
	id := fmt.Sprintf("%s:%d:%s", taskID, version, event)
	if event == workflow.StageCompleted && cause != "" {
		id = fmt.Sprintf("%s:%d:completed:%s", taskID, version, cause)
	}
	if cause == "" {
		cause = id
	}
	root := cause
	if len(correlation) > 0 && correlation[0] != "" {
		root = correlation[0]
	}
	return q.EnqueueWorkflowEvent(ctx, sqlc.EnqueueWorkflowEventParams{ID: id, WorkspaceID: workspaceID, TaskID: taskID, ActorID: actorID, StageVersion: version, EventType: string(event), CausationID: cause, CorrelationID: root})
}

func enqueueStageEntry(ctx context.Context, q *sqlc.Queries, workspaceID, taskID, actorID string, version int64, cause string, correlation ...string) error {
	return enqueueTaskEvent(ctx, q, workspaceID, taskID, actorID, version, workflow.StageEntered, cause, correlation...)
}

// WithWorkflowEvent owns one transaction from claim through acknowledgement.
// Only database effects occur here; external execution uses the command outbox.
func (r *TaskPostgresRepository) WithWorkflowEvent(ctx context.Context, decide func(task.WorkflowEventSnapshot) (task.WorkflowEventDecision, error)) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	e, err := q.PendingWorkflowEvent(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	entry, err := q.TaskWorkflowState(ctx, sqlc.TaskWorkflowStateParams{WorkspaceID: e.WorkspaceID, ID: e.TaskID})
	if err != nil {
		return false, err
	}
	row, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: e.WorkspaceID, ID: entry.WorkflowID})
	if err != nil {
		return false, err
	}
	d, err := workflowDefinition(ctx, q, row)
	if err != nil {
		return false, err
	}
	state, err := q.LockWorkflowTask(ctx, sqlc.LockWorkflowTaskParams{WorkspaceID: e.WorkspaceID, ID: e.TaskID})
	if err != nil {
		return false, err
	}
	active := false
	if _, err = q.ActiveTaskLaunch(ctx, sqlc.ActiveTaskLaunchParams{WorkspaceID: e.WorkspaceID, TaskID: e.TaskID}); err == nil {
		active = true
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	decision, err := decide(task.WorkflowEventSnapshot{
		Workflow: d, Event: task.WorkflowEvent{Type: workflow.EventType(e.EventType), StageVersion: e.StageVersion, RunID: e.RunID, ActorID: e.ActorID},
		StageID: state.Status, AssigneeAgentID: state.Assignee, RunID: state.WorkflowRunID, StageVersion: state.StageVersion, WorkState: task.WorkState(state.StageWorkState), ActiveLaunch: active,
	})
	if err != nil {
		return false, err
	}
	if decision.Plan != nil {
		if err = applyWorkflowPlan(ctx, q, e, decision.ActorID, *decision.Plan, state.Status); err != nil {
			return false, err
		}
	}
	if decision.WorkState != "" {
		if err = q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: e.WorkspaceID, ID: e.TaskID, StageVersion: e.StageVersion, StageWorkState: string(decision.WorkState)}); err != nil {
			return false, err
		}
	}
	if err = q.AcknowledgeWorkflowEvent(ctx, sqlc.AcknowledgeWorkflowEventParams{ID: e.ID, ConfigurationVersion: pgtype.Int8{Int64: d.Version, Valid: true}, MatchedRuleID: pgtype.Text{String: decision.RuleID, Valid: decision.RuleID != ""}}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func applyWorkflowPlan(ctx context.Context, q *sqlc.Queries, e sqlc.WorkflowEvent, actor string, plan workflow.Plan, current string) error {
	version := e.StageVersion
	moved := plan.StageID != current
	if moved {
		if err := q.MoveWorkflowTask(ctx, sqlc.MoveWorkflowTaskParams{WorkspaceID: e.WorkspaceID, ID: e.TaskID, StageVersion: version, Status: plan.StageID, Column5: plan.AssigneeAgentID}); err != nil {
			return err
		}
		version++
	}
	return persistWorkflowEffects(ctx, q, e.WorkspaceID, e.TaskID, actor, version, moved, plan, task.TransitionInput{CommandID: e.ID + ":start", SessionMode: "new"}, e.ID, e.CorrelationID)
}
