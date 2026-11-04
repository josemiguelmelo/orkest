package postgres

import (
	"context"
	"fmt"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/identity"
)

type UserPostgresRepository struct {
	queries *sqlc.Queries
}

func NewUserPostgresRepository(database *PostgresDatabase) *UserPostgresRepository {
	return &UserPostgresRepository{queries: sqlc.New(database.pool)}
}

var _ identity.UserRegistrationRepository = (*UserPostgresRepository)(nil)

func (s *UserPostgresRepository) RegisterUser(ctx context.Context, subject, username, email string) (identity.User, error) {
	id, err := ids.NewUUID()
	if err != nil {
		return identity.User{}, fmt.Errorf("generate user ID: %w", err)
	}
	row, err := s.queries.RegisterUser(ctx, sqlc.RegisterUserParams{ID: id, KeycloakSubject: subject, Username: username, Email: email})
	return identity.User{ID: row.ID, Username: row.Username, Email: row.Email}, err
}

func (s *UserPostgresRepository) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return s.queries.IsPlatformAdmin(ctx, userID)
}
