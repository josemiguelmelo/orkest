package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/workflow"
)

func enqueueStageStart(ctx context.Context, q *sqlc.Queries, workspaceID, taskID, actorID string, version int64, in task.TaskInput) error {
	if actorID == "" {
		return q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: workspaceID, ID: taskID, StageVersion: version, StageWorkState: string(task.WorkStateFailed)})
	}
	if in.CommandID == "" {
		id, err := ids.NewUUID()
		if err != nil {
			return err
		}
		in.CommandID = id
	}
	options, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if err = q.EnqueueWorkflowCommand(ctx, sqlc.EnqueueWorkflowCommandParams{ID: in.CommandID, WorkspaceID: workspaceID, TaskID: taskID, StageVersion: version, ActorID: actorID, Options: options}); err != nil {
		return mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: workspaceID, ID: taskID, StageVersion: version, StageWorkState: string(task.WorkStateQueued)})
}

func (r *TaskPostgresRepository) TransitionTask(ctx context.Context, userID, orgID, workspaceID, id string, in task.TaskInput) (task.Task, error) {
	if in.Labels == nil {
		in.Labels = []string{}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return task.Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	entry, err := q.TaskWorkflowState(ctx, sqlc.TaskWorkflowStateParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	data, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: workspaceID, ID: entry.WorkflowID})
	if err != nil {
		return task.Task{}, err
	}
	d, err := decodeWorkflow(data)
	if err != nil {
		return task.Task{}, err
	}
	state, err := q.LockWorkflowTask(ctx, sqlc.LockWorkflowTaskParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	if state.WorkflowID != entry.WorkflowID {
		return task.Task{}, modelerrors.ErrConflict
	}
	if in.CommandID != "" {
		previous, e := q.GetWorkflowCommand(ctx, sqlc.GetWorkflowCommandParams{WorkspaceID: workspaceID, TaskID: id, ID: in.CommandID})
		if e == nil {
			var original task.TaskInput
			if e = json.Unmarshal(previous.Options, &original); e != nil {
				return task.Task{}, e
			}
			if original.StartWork != in.StartWork || original.CompleteStage != in.CompleteStage || original.StageVersion != in.StageVersion || original.SessionMode != in.SessionMode || original.PromptText != in.PromptText || (!in.StartWork && !in.CompleteStage && original.Status != in.Status) {
				return task.Task{}, modelerrors.ErrConflict
			}
			return r.Task(ctx, orgID, workspaceID, id)
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return task.Task{}, e
		}
	}
	if in.StageVersion != 0 && in.StageVersion != state.StageVersion {
		return task.Task{}, modelerrors.ErrConflict
	}
	trigger := workflow.Enter
	changing := state.Status != in.Status
	snapshot, e := decodeWorkflow(state.WorkflowSnapshot)
	if e != nil {
		return task.Task{}, e
	}
	if changing && state.Status == snapshot.FinalStageID() {
		return task.Task{}, modelerrors.ErrConflict
	}
	if in.CompleteStage {
		trigger = workflow.Complete
	} else if in.StartWork {
		trigger = workflow.Start
	}
	if changing && (in.CompleteStage || in.StartWork) {
		trigger = workflow.Enter
	}
	if changing || in.StartWork || in.CompleteStage || state.Assignee != in.AssigneeAgentID {
		if state.StageWorkState == string(task.WorkStateQueued) || state.StageWorkState == string(task.WorkStateRunning) {
			return task.Task{}, modelerrors.ErrConflict
		}
		if _, e := q.ActiveTaskLaunch(ctx, sqlc.ActiveTaskLaunchParams{WorkspaceID: workspaceID, TaskID: id}); e == nil {
			return task.Task{}, modelerrors.ErrConflict
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return task.Task{}, e
		}
	}
	plan := workflow.Plan{StageID: in.Status, AssigneeAgentID: in.AssigneeAgentID}
	if changing || in.StartWork || in.CompleteStage {
		definition := d
		if in.CompleteStage || in.StartWork {
			definition, err = decodeWorkflow(state.WorkflowSnapshot)
			if err != nil {
				return task.Task{}, err
			}
		}
		plan, err = workflow.PlanTransition(definition, state.Status, in.Status, in.AssigneeAgentID, trigger)
		if err != nil {
			return task.Task{}, err
		}
		if plan.StageID != state.Status {
			// Entry always uses the current stage configuration.
			plan, err = workflow.PlanTransition(d, state.Status, plan.StageID, plan.AssigneeAgentID, workflow.Enter)
			if err != nil {
				return task.Task{}, err
			}
		}
	}
	row, err := q.GetTask(ctx, sqlc.GetTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return task.Task{}, err
	}
	if row.SourceProvider != "manual" {
		in.Title, in.Description, in.Labels = row.Title, row.Description, row.Labels
	}
	_, err = q.UpdateTask(ctx, sqlc.UpdateTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Title: in.Title, Description: in.Description, Status: plan.StageID, Column7: plan.AssigneeAgentID, Labels: in.Labels})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	if err = q.DeleteTaskRepositories(ctx, sqlc.DeleteTaskRepositoriesParams{WorkspaceID: workspaceID, TaskID: id}); err != nil {
		return task.Task{}, err
	}
	for _, repoID := range in.RepositoryIDs {
		if err = q.InsertTaskRepository(ctx, sqlc.InsertTaskRepositoryParams{WorkspaceID: workspaceID, TaskID: id, RepositoryID: repoID}); err != nil {
			return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
		}
	}
	version := state.StageVersion
	if plan.StageID != state.Status {
		version++
	}
	if plan.Start {
		if err = enqueueStageStart(ctx, q, workspaceID, id, userID, version, in); err != nil {
			return task.Task{}, err
		}
	} else if plan.Completed {
		if err = q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: workspaceID, ID: id, StageVersion: version, StageWorkState: string(task.WorkStateCompleted)}); err != nil {
			return task.Task{}, err
		}
	}
	if !plan.Start && in.CommandID != "" {
		options, e := json.Marshal(in)
		if e != nil {
			return task.Task{}, e
		}
		if e = q.RecordWorkflowTransition(ctx, sqlc.RecordWorkflowTransitionParams{ID: in.CommandID, WorkspaceID: workspaceID, TaskID: id, StageVersion: version, ActorID: userID, Options: options}); e != nil {
			return task.Task{}, mapError(e, modelerrors.ErrConflict, modelerrors.ErrConflict)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return r.Task(ctx, orgID, workspaceID, id)
}

// completeWorkflowRun executes in the same transaction as terminal run status.
func completeWorkflowRun(ctx context.Context, q *sqlc.Queries, runID string, success bool) error {
	run, err := q.RunWorkflowEntry(ctx, runID)
	if err != nil {
		return err
	}
	data, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: run.WorkspaceID, ID: run.WorkflowID})
	if err != nil {
		return err
	}
	d, err := decodeWorkflow(data)
	if err != nil {
		return err
	}
	state, err := q.LockWorkflowTask(ctx, sqlc.LockWorkflowTaskParams{WorkspaceID: run.WorkspaceID, ID: run.TaskID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if state.StageVersion != run.StageVersion || state.WorkflowRunID != runID || state.StageWorkState == string(task.WorkStateCompleted) {
		return nil
	}
	workState := task.WorkStateFailed
	if success {
		workState = task.WorkStateCompleted
		snapshot, e := decodeWorkflow(state.WorkflowSnapshot)
		if e != nil {
			return e
		}
		plan, e := workflow.PlanTransition(snapshot, state.Status, "", state.Assignee, workflow.Succeeded)
		if errors.Is(e, modelerrors.ErrConflict) {
			return q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: run.WorkspaceID, ID: run.TaskID, StageVersion: state.StageVersion, StageWorkState: string(task.WorkStateCompleted)})
		}
		if e != nil {
			return e
		}
		if plan.StageID != state.Status {
			if _, exists := d.Stage(plan.StageID); exists {
				plan, e = workflow.PlanTransition(d, state.Status, plan.StageID, plan.AssigneeAgentID, workflow.Enter)
				if errors.Is(e, modelerrors.ErrConflict) {
					return q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: run.WorkspaceID, ID: run.TaskID, StageVersion: state.StageVersion, StageWorkState: string(task.WorkStateCompleted)})
				}
				if e != nil {
					return e
				}
				if e = q.MoveWorkflowTask(ctx, sqlc.MoveWorkflowTaskParams{WorkspaceID: run.WorkspaceID, ID: run.TaskID, StageVersion: state.StageVersion, Status: plan.StageID, Column5: plan.AssigneeAgentID}); e != nil {
					return e
				}
				if plan.Start {
					return enqueueStageStart(ctx, q, run.WorkspaceID, run.TaskID, run.ActorID.String, state.StageVersion+1, task.TaskInput{SessionMode: "new"})
				}
				return nil
			}
		}
	}
	return q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: run.WorkspaceID, ID: run.TaskID, StageVersion: state.StageVersion, StageWorkState: string(workState)})
}
