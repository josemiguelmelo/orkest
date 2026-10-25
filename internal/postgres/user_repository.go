package postgres

import (
	"context"
	"fmt"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	"github.com/josemiguelmelo/orkest/internal/identity"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
)

type UserPostgresRepository struct {
	queries *sqlc.Queries
}

func NewUserPostgresRepository(database *PostgresDatabase) *UserPostgresRepository {
	return &UserPostgresRepository{queries: sqlc.New(database.pool)}
}

var _ identity.Repository = (*UserPostgresRepository)(nil)

func (s *UserPostgresRepository) RegisterUser(ctx context.Context, subject, username, email string) (identity.User, error) {
	id, err := ids.NewUUID()
	if err != nil {
		return identity.User{}, fmt.Errorf("generate user ID: %w", err)
	}
	row, err := s.queries.RegisterUser(ctx, sqlc.RegisterUserParams{ID: id, KeycloakSubject: subject, Username: username, Email: email})
	return identity.User{ID: row.ID, Username: row.Username, Email: row.Email}, err
}
