package postgres

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/workspace"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type WorkspacePostgresRepository struct {
	queries *sqlc.Queries
}

func NewWorkspacePostgresRepository(database *PostgresDatabase) *WorkspacePostgresRepository {
	return &WorkspacePostgresRepository{queries: sqlc.New(database.pool)}
}

var _ workspace.WorkspaceRepository = (*WorkspacePostgresRepository)(nil)

func (s *WorkspacePostgresRepository) Workspaces(ctx context.Context, orgID, userID string) ([]workspacemodel.Workspace, error) {
	rows, err := s.queries.ListWorkspaces(ctx, sqlc.ListWorkspacesParams{OrganizationID: orgID, UserID: userID})
	if err != nil {
		return nil, err
	}
	out := make([]workspacemodel.Workspace, 0, len(rows))
	for _, row := range rows {
		out = append(out, workspacemodel.Workspace{ID: row.ID, OrganizationID: row.OrganizationID, Name: row.Name, Role: row.Role})
	}
	return out, nil
}

func (s *WorkspacePostgresRepository) Workspace(ctx context.Context, orgID, id, userID string) (workspacemodel.Workspace, error) {
	row, err := s.queries.GetWorkspace(ctx, sqlc.GetWorkspaceParams{OrganizationID: orgID, ID: id, UserID: userID})
	w := workspacemodel.Workspace{ID: row.ID, OrganizationID: row.OrganizationID, Name: row.Name, Role: row.Role}
	return w, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (s *WorkspacePostgresRepository) CreateWorkspace(ctx context.Context, id, orgID, name string) (workspacemodel.Workspace, error) {
	row, err := s.queries.InsertWorkspace(ctx, sqlc.InsertWorkspaceParams{ID: id, OrganizationID: orgID, Name: name})
	w := workspacemodel.Workspace{ID: row.ID, OrganizationID: row.OrganizationID, Name: row.Name}
	return w, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (s *WorkspacePostgresRepository) UpdateWorkspace(ctx context.Context, orgID, id, name string) (workspacemodel.Workspace, error) {
	row, err := s.queries.UpdateWorkspace(ctx, sqlc.UpdateWorkspaceParams{OrganizationID: orgID, ID: id, Name: name})
	w := workspacemodel.Workspace{ID: row.ID, OrganizationID: row.OrganizationID, Name: row.Name}
	return w, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (s *WorkspacePostgresRepository) DeleteWorkspace(ctx context.Context, orgID, id string) error {
	affected, err := s.queries.DeleteWorkspace(ctx, sqlc.DeleteWorkspaceParams{OrganizationID: orgID, ID: id})
	if err == nil && affected == 0 {
		return modelerrors.ErrNotFound
	}
	return err
}

func (s *WorkspacePostgresRepository) WorkspaceMembers(ctx context.Context, orgID, workspaceID string) ([]workspacemodel.Membership, error) {
	rows, err := s.queries.ListWorkspaceMembers(ctx, sqlc.ListWorkspaceMembersParams{OrganizationID: orgID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]workspacemodel.Membership, 0, len(rows))
	for _, row := range rows {
		m := workspacemodel.Membership{Role: row.Role}
		m.User.ID, m.User.Username, m.User.Email = row.ID, row.Username, row.Email
		out = append(out, m)
	}
	return out, nil
}

func (s *WorkspacePostgresRepository) SetWorkspaceMember(ctx context.Context, orgID, workspaceID, userID, role string) error {
	err := s.queries.UpsertWorkspaceMember(ctx, sqlc.UpsertWorkspaceMemberParams{OrganizationID: orgID, WorkspaceID: workspaceID, UserID: userID, Role: role})
	return mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (s *WorkspacePostgresRepository) RemoveWorkspaceMember(ctx context.Context, orgID, workspaceID, userID string) error {
	affected, err := s.queries.DeleteWorkspaceMember(ctx, sqlc.DeleteWorkspaceMemberParams{OrganizationID: orgID, WorkspaceID: workspaceID, UserID: userID})
	if err == nil && affected == 0 {
		return modelerrors.ErrNotFound
	}
	return err
}
