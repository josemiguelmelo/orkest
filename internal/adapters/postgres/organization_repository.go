package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/organization"
	organizationmodel "github.com/josemiguelmelo/orkest/internal/server/organization/model"
)

type OrganizationPostgresRepository struct {
	database *PostgresDatabase
	queries  *sqlc.Queries
}

func NewOrganizationPostgresRepository(database *PostgresDatabase) *OrganizationPostgresRepository {
	return &OrganizationPostgresRepository{database: database, queries: sqlc.New(database.pool)}
}

var _ organization.OrganizationRepository = (*OrganizationPostgresRepository)(nil)

func (s *OrganizationPostgresRepository) Organizations(ctx context.Context, userID string) ([]organizationmodel.Organization, error) {
	rows, err := s.queries.ListOrganizations(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]organizationmodel.Organization, 0, len(rows))
	for _, row := range rows {
		result = append(result, organizationmodel.Organization{ID: row.ID, Name: row.Name, Role: row.Role})
	}
	return result, nil
}

func (s *OrganizationPostgresRepository) Organization(ctx context.Context, userID, orgID string) (organizationmodel.Organization, error) {
	row, err := s.queries.GetOrganization(ctx, sqlc.GetOrganizationParams{ID: orgID, UserID: userID})
	o := organizationmodel.Organization{ID: row.ID, Name: row.Name, Role: row.Role}
	return o, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (s *OrganizationPostgresRepository) CreateOrganization(ctx context.Context, id, userID, name string) (organizationmodel.Organization, error) {
	tx, err := s.database.pool.Begin(ctx)
	if err != nil {
		return organizationmodel.Organization{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)
	row, err := qtx.InsertOrganization(ctx, sqlc.InsertOrganizationParams{ID: id, Name: name})
	o := organizationmodel.Organization{ID: row.ID, Name: row.Name}
	if err != nil {
		return o, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	err = qtx.InsertOrganizationOwner(ctx, sqlc.InsertOrganizationOwnerParams{OrganizationID: id, UserID: userID})
	if err != nil {
		return o, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	if err = tx.Commit(ctx); err != nil {
		return o, err
	}
	o.Role = "owner"
	return o, nil
}

func (s *OrganizationPostgresRepository) UpdateOrganization(ctx context.Context, id, userID, name string) (organizationmodel.Organization, error) {
	row, err := s.queries.UpdateOrganization(ctx, sqlc.UpdateOrganizationParams{ID: id, UserID: userID, Name: name})
	o := organizationmodel.Organization{ID: row.ID, Name: row.Name, Role: row.Role}
	return o, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (s *OrganizationPostgresRepository) DeleteOrganization(ctx context.Context, id string) error {
	affected, err := s.queries.DeleteOrganization(ctx, id)
	if err == nil && affected == 0 {
		return modelerrors.ErrNotFound
	}
	return err
}

func (s *OrganizationPostgresRepository) OrganizationRole(ctx context.Context, userID, orgID string) (string, error) {
	role, err := s.queries.GetOrganizationRole(ctx, sqlc.GetOrganizationRoleParams{UserID: userID, OrganizationID: orgID})
	return role, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (s *OrganizationPostgresRepository) OrganizationMembers(ctx context.Context, orgID string) ([]organizationmodel.Membership, error) {
	rows, err := s.queries.ListOrganizationMembers(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]organizationmodel.Membership, 0, len(rows))
	for _, row := range rows {
		m := organizationmodel.Membership{Role: row.Role}
		m.User.ID, m.User.Username, m.User.Email = row.ID, row.Username, row.Email
		out = append(out, m)
	}
	return out, nil
}

func (s *OrganizationPostgresRepository) SetOrganizationMember(ctx context.Context, actorID, orgID, userID, role string) error {
	tx, err := s.database.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)
	if _, err = qtx.LockOrganization(ctx, orgID); errors.Is(err, pgx.ErrNoRows) {
		return modelerrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	actorRole, err := qtx.GetOrganizationMemberRole(ctx, sqlc.GetOrganizationMemberRoleParams{OrganizationID: orgID, UserID: actorID})
	if errors.Is(err, pgx.ErrNoRows) {
		return modelerrors.ErrForbidden
	}
	if err != nil {
		return err
	}
	if actorRole != "owner" && actorRole != "admin" {
		return modelerrors.ErrForbidden
	}
	if actorRole == "admin" && role == "owner" {
		return modelerrors.ErrForbidden
	}
	oldRole, err := qtx.LockOrganizationMemberRole(ctx, sqlc.LockOrganizationMemberRoleParams{OrganizationID: orgID, UserID: userID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if actorRole == "admin" && oldRole == "owner" {
		return modelerrors.ErrForbidden
	}
	if oldRole == "owner" && role != "owner" {
		count, err := qtx.CountOrganizationOwners(ctx, orgID)
		if err != nil {
			return err
		}
		if count <= 1 {
			return modelerrors.ErrConflict
		}
	}
	err = qtx.UpsertOrganizationMember(ctx, sqlc.UpsertOrganizationMemberParams{OrganizationID: orgID, UserID: userID, Role: role})
	if err != nil {
		return mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return tx.Commit(ctx)
}

func (s *OrganizationPostgresRepository) RemoveOrganizationMember(ctx context.Context, actorID, orgID, userID string) error {
	tx, err := s.database.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)
	if _, err = qtx.LockOrganization(ctx, orgID); errors.Is(err, pgx.ErrNoRows) {
		return modelerrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	actorRole, err := qtx.GetOrganizationMemberRole(ctx, sqlc.GetOrganizationMemberRoleParams{OrganizationID: orgID, UserID: actorID})
	if errors.Is(err, pgx.ErrNoRows) {
		return modelerrors.ErrForbidden
	}
	if err != nil {
		return err
	}
	if actorRole != "owner" && actorRole != "admin" {
		return modelerrors.ErrForbidden
	}
	role, err := qtx.LockOrganizationMemberRole(ctx, sqlc.LockOrganizationMemberRoleParams{OrganizationID: orgID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return modelerrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if role == "owner" {
		if actorRole == "admin" {
			return modelerrors.ErrForbidden
		}
		count, err := qtx.CountOrganizationOwners(ctx, orgID)
		if err != nil {
			return err
		}
		if count <= 1 {
			return modelerrors.ErrConflict
		}
	}
	_, err = qtx.DeleteOrganizationMember(ctx, sqlc.DeleteOrganizationMemberParams{OrganizationID: orgID, UserID: userID})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
