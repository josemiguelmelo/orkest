package postgres

import (
	"context"
	"encoding/json"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
)

func persistStageEvent(ctx context.Context, q *sqlc.Queries, workspaceID, taskID, actorID string, event workflow.EventIntent) error {
	return q.EnqueueWorkflowEvent(ctx, sqlc.EnqueueWorkflowEventParams{PromptText: event.PromptText, ID: event.ID, WorkspaceID: workspaceID, TaskID: taskID, ActorID: actorID, StageVersion: event.StageVersion, EventType: string(event.Type), RunID: event.RunID, CausationID: event.CausationID, CorrelationID: event.CorrelationID})
}

func persistStageEffects(ctx context.Context, q *sqlc.Queries, workspaceID, taskID, actorID string, effects task.WorkflowPersistenceEffects) error {
	for _, event := range effects.Events {
		if err := persistStageEvent(ctx, q, workspaceID, taskID, actorID, event); err != nil {
			return err
		}
	}
	if effects.Execution != nil {
		options, err := json.Marshal(effects.Execution.Options)
		if err != nil {
			return err
		}
		if err = q.EnqueueWorkflowCommand(ctx, sqlc.EnqueueWorkflowCommandParams{ID: effects.Execution.ID, WorkspaceID: workspaceID, TaskID: taskID, ActorID: actorID, StageVersion: effects.StageVersion, Options: options}); err != nil {
			return err
		}
	}
	if effects.WorkState != "" {
		if err := q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: workspaceID, ID: taskID, StageVersion: effects.StageVersion, StageWorkState: string(effects.WorkState)}); err != nil {
			return err
		}
	}
	if effects.History != nil {
		options, err := json.Marshal(effects.History)
		if err != nil {
			return err
		}
		if err = q.RecordWorkflowTransition(ctx, sqlc.RecordWorkflowTransitionParams{ID: effects.History.CommandID, WorkspaceID: workspaceID, TaskID: taskID, ActorID: actorID, StageVersion: effects.StageVersion, Options: options}); err != nil {
			return err
		}
	}
	return nil
}
