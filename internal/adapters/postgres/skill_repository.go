package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/skill"
)

type SkillCatalogPostgresRepository struct{ queries *sqlc.Queries }

func NewSkillCatalogPostgresRepository(database *PostgresDatabase) *SkillCatalogPostgresRepository {
	return &SkillCatalogPostgresRepository{queries: sqlc.New(database.pool)}
}

var _ skill.CatalogRepository = (*SkillCatalogPostgresRepository)(nil)

func (r *SkillCatalogPostgresRepository) Skills(ctx context.Context, orgID, workspaceID string) ([]skill.Skill, error) {
	rows, err := r.queries.ListVisibleSkills(ctx, sqlc.ListVisibleSkillsParams{OrganizationID: nullableText(orgID), WorkspaceID: nullableText(workspaceID)})
	if err != nil {
		return nil, err
	}
	out := make([]skill.Skill, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromSQLSkill(row))
	}
	return out, nil
}

func (r *SkillCatalogPostgresRepository) Skill(ctx context.Context, orgID, workspaceID, id string) (skill.Skill, error) {
	row, err := r.queries.GetVisibleSkill(ctx, sqlc.GetVisibleSkillParams{OrganizationID: nullableText(orgID), WorkspaceID: nullableText(workspaceID), ID: id})
	if err != nil {
		return skill.Skill{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return fromSQLSkill(row), nil
}

func (r *SkillCatalogPostgresRepository) CreateSkill(ctx context.Context, id string, scope skill.Scope, orgID, workspaceID, skillMD string) (skill.Skill, error) {
	row, err := r.queries.InsertSkill(ctx, sqlc.InsertSkillParams{ID: id, Scope: string(scope), Column3: orgID, Column4: workspaceID, SkillMd: skillMD})
	if err != nil {
		return skill.Skill{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return fromSQLSkill(row), nil
}

func (r *SkillCatalogPostgresRepository) UpdateSkill(ctx context.Context, id, skillMD string) (skill.Skill, error) {
	row, err := r.queries.UpdateSkill(ctx, sqlc.UpdateSkillParams{ID: id, SkillMd: skillMD})
	if err != nil {
		return skill.Skill{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return fromSQLSkill(row), nil
}

func (r *SkillCatalogPostgresRepository) DeleteSkill(ctx context.Context, id string) error {
	count, err := r.queries.DeleteSkill(ctx, id)
	if err == nil && count == 0 {
		return modelerrors.ErrNotFound
	}
	return err
}

func (r *SkillCatalogPostgresRepository) VisibleSkills(ctx context.Context, orgID, workspaceID string, ids []string) ([]skill.Skill, error) {
	out := make([]skill.Skill, 0, len(ids))
	for _, id := range ids {
		row, err := r.queries.GetVisibleSkill(ctx, sqlc.GetVisibleSkillParams{OrganizationID: nullableText(orgID), WorkspaceID: nullableText(workspaceID), ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, fromSQLSkill(row))
	}
	return out, nil
}

func fromSQLSkill(value sqlc.Skill) skill.Skill {
	result := skill.Skill{ID: value.ID, Scope: skill.Scope(value.Scope), SkillMD: value.SkillMd, CreatedAt: value.CreatedAt.Time, UpdatedAt: value.UpdatedAt.Time}
	if value.OrganizationID.Valid {
		result.OrganizationID = value.OrganizationID.String
	}
	if value.WorkspaceID.Valid {
		result.WorkspaceID = value.WorkspaceID.String
	}
	return result
}

func nullableText(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
