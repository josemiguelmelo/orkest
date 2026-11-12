package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

// guardIntegrationConfiguration holds configuration locks through run reservation.
// A callback admitted before a disable, rule edit or authority revocation cannot
// launch against newer configuration. Lock order is workflow, connection, task.
func guardIntegrationConfiguration(ctx context.Context, q *sqlc.Queries, config orchestrationmodel.RunConfig) error {
	workflow, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: config.WorkspaceID, ID: config.IntegrationWorkflowID})
	if err != nil {
		return mapError(err, problem.ErrConflict, problem.ErrConflict)
	}
	if workflow.Version != config.IntegrationGuard.ConfigurationVersion {
		return problem.ErrConflict
	}
	connection, err := q.LockWorkflowStageIntegration(ctx, config.IntegrationGuard.ConnectionID)
	if err != nil {
		return mapError(err, problem.ErrConflict, problem.ErrConflict)
	}
	if connection.WorkspaceID != config.WorkspaceID || !connection.Enabled || connection.Version != config.IntegrationGuard.ConnectionVersion {
		return problem.ErrConflict
	}
	workspace, err := q.GetWorkspace(ctx, sqlc.GetWorkspaceParams{OrganizationID: config.OrganizationID, ID: config.WorkspaceID, UserID: config.IntegrationActorID})
	if errors.Is(err, pgx.ErrNoRows) {
		return problem.ErrForbidden
	}
	if err != nil {
		return err
	}
	if workspace.Role != "admin" && workspace.Role != "owner" {
		return problem.ErrForbidden
	}
	return nil
}
