package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
)

// WithAutomationEvent owns one transaction from claim through acknowledgement.
// Only database effects occur here; external execution uses the command outbox.
func (r *AutomationEventPostgresRepository) WithAutomationEvent(ctx context.Context, decide func(workflowautomation.AutomationEventSnapshot) (workflowautomation.AutomationEventEffects, error)) (bool, error) {
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
	var integration *workflowmodel.IntegrationEvent
	integrationAuthorized := false
	integrationConfigurationValid := false
	if e.EventType == string(workflowmodel.IntegrationAction) {
		if err = json.Unmarshal(e.Integration, &integration); err != nil {
			return false, err
		}
		if integration != nil {
			connection, connErr := q.LockWorkflowStageIntegration(ctx, integration.ConnectionID)
			if connErr != nil && !errors.Is(connErr, pgx.ErrNoRows) {
				return false, connErr
			}
			if connErr == nil && connection.WorkspaceID == e.WorkspaceID && connection.Enabled && connection.Version == integration.ConnectionVersion {
				integrationConfigurationValid = true
				organizationID, scopeErr := q.WorkflowWorkspaceOrganization(ctx, e.WorkspaceID)
				if scopeErr != nil {
					return false, scopeErr
				}
				workspace, accessErr := q.GetWorkspace(ctx, sqlc.GetWorkspaceParams{OrganizationID: organizationID, ID: e.WorkspaceID, UserID: e.ActorID})
				if accessErr != nil && !errors.Is(accessErr, pgx.ErrNoRows) {
					return false, accessErr
				}
				integrationAuthorized = accessErr == nil && (workspace.Role == "owner" || workspace.Role == "admin")
			}
		}
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
	decision, err := decide(workflowautomation.AutomationEventSnapshot{
		Workflow: d, Event: workflowautomation.AutomationEvent{ID: e.ID, CorrelationID: e.CorrelationID, WorkspaceID: e.WorkspaceID, TaskID: e.TaskID, Type: workflowmodel.EventType(e.EventType), StageVersion: e.StageVersion, RunID: e.RunID, ActorID: e.ActorID, Integration: integration},
		StageID: state.Status, AssigneeAgentID: state.Assignee, RunID: state.WorkflowRunID, StageVersion: state.StageVersion, WorkState: taskmodel.WorkState(state.StageWorkState), ActiveLaunch: active, IntegrationAuthorized: integrationAuthorized, IntegrationConfigurationValid: integrationConfigurationValid,
	})
	if err != nil {
		return false, err
	}
	if decision.Plan != nil {
		if decision.Moved {
			if err = q.MoveWorkflowTask(ctx, sqlc.MoveWorkflowTaskParams{WorkspaceID: e.WorkspaceID, ID: e.TaskID, StageVersion: e.StageVersion, Status: decision.Plan.StageID, Column5: decision.Plan.AssigneeAgentID}); err != nil {
				return false, err
			}
		}
		if err = persistStageEffects(ctx, q, e.WorkspaceID, e.TaskID, decision.ActorID, decision.Persistence); err != nil {
			return false, err
		}
	}
	if decision.WorkState != "" {
		if err = q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: e.WorkspaceID, ID: e.TaskID, StageVersion: e.StageVersion, StageWorkState: string(decision.WorkState)}); err != nil {
			return false, err
		}
	}
	if err = q.AcknowledgeWorkflowEvent(ctx, sqlc.AcknowledgeWorkflowEventParams{ID: decision.AcknowledgeID, ConfigurationVersion: pgtype.Int8{Int64: decision.ConfigurationVersion, Valid: true}, MatchedRuleID: pgtype.Text{String: decision.RuleID, Valid: decision.RuleID != ""}, ProcessingError: decision.FailureReason}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

type AutomationEventPostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewAutomationEventPostgresRepository(database *PostgresDatabase) *AutomationEventPostgresRepository {
	return &AutomationEventPostgresRepository{pool: database.pool, queries: sqlc.New(database.pool)}
}
