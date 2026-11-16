package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/identity"
	identitymodel "github.com/josemiguelmelo/orkest/internal/server/identity/model"
)

type UserPostgresRepository struct {
	queries *sqlc.Queries
}

func NewUserPostgresRepository(database *PostgresDatabase) *UserPostgresRepository {
	return &UserPostgresRepository{queries: sqlc.New(database.pool)}
}

var (
	_ identity.UserRegistrationRepository        = (*UserPostgresRepository)(nil)
	_ identity.LocalAccountRepository            = (*UserPostgresRepository)(nil)
	_ identity.PlatformAdminManagementRepository = (*UserPostgresRepository)(nil)
)

func (s *UserPostgresRepository) RegisterUser(ctx context.Context, subject, username, email string) (identitymodel.User, error) {
	id, err := ids.NewUUID()
	if err != nil {
		return identitymodel.User{}, fmt.Errorf("generate user ID: %w", err)
	}
	row, err := s.queries.RegisterUser(ctx, sqlc.RegisterUserParams{ID: id, KeycloakSubject: pgtype.Text{String: subject, Valid: true}, Username: username, Email: email})
	return identitymodel.User{ID: row.ID, Username: row.Username, Email: row.Email}, err
}

func (s *UserPostgresRepository) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return s.queries.IsPlatformAdmin(ctx, userID)
}

func (s *UserPostgresRepository) PromotePlatformAdmin(ctx context.Context, userID string) (int64, error) {
	count, err := s.queries.PromotePlatformAdmin(ctx, userID)
	return count, err
}

func (s *UserPostgresRepository) ProvisionLocalAccount(ctx context.Context, id, orgID, username, normalizedUsername, email, normalizedEmail, passwordHash, role string) (identitymodel.User, error) {
	row, err := s.queries.ProvisionLocalAccount(ctx, sqlc.ProvisionLocalAccountParams{ID: id, Username: username, Email: email, NormalizedUsername: normalizedUsername, NormalizedEmail: normalizedEmail, PasswordHash: passwordHash, OrganizationID: orgID, Role: role})
	if err != nil {
		return identitymodel.User{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return identitymodel.User{ID: row.ID, Username: row.Username, Email: row.Email}, nil
}

func (s *UserPostgresRepository) BootstrapLocalAdminRequired(ctx context.Context, normalizedUsername string) (bool, error) {
	return s.queries.BootstrapLocalAdminRequired(ctx, normalizedUsername)
}

func (s *UserPostgresRepository) BootstrapLocalAdmin(ctx context.Context, id, username, normalizedUsername, email, normalizedEmail, passwordHash string) (identitymodel.User, bool, error) {
	row, err := s.queries.BootstrapLocalAdmin(ctx, sqlc.BootstrapLocalAdminParams{ID: id, Username: username, NormalizedUsername: normalizedUsername, Email: email, NormalizedEmail: normalizedEmail, PasswordHash: passwordHash})
	if errors.Is(err, pgx.ErrNoRows) {
		return identitymodel.User{}, false, nil
	}
	if err != nil {
		return identitymodel.User{}, false, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return identitymodel.User{ID: row.ID, Username: row.Username, Email: row.Email}, true, nil
}

func (s *UserPostgresRepository) LocalCredentialByUsername(ctx context.Context, username string) (identity.LocalCredential, error) {
	row, err := s.queries.LocalCredentialByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.LocalCredential{}, identity.ErrInvalidCredentials
		}
		return identity.LocalCredential{}, err
	}
	return identity.LocalCredential{User: identitymodel.User{ID: row.ID, Username: row.Username, Email: row.Email}, PasswordHash: row.PasswordHash}, nil
}

func (s *UserPostgresRepository) LocalCredentialByUserID(ctx context.Context, userID string) (identity.LocalCredential, error) {
	row, err := s.queries.LocalCredentialByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.LocalCredential{}, identity.ErrNoLocalPassword
		}
		return identity.LocalCredential{}, err
	}
	return identity.LocalCredential{User: identitymodel.User{ID: row.ID, Username: row.Username, Email: row.Email}, PasswordHash: row.PasswordHash}, nil
}

func (s *UserPostgresRepository) HasLocalPassword(ctx context.Context, userID string) (bool, error) {
	return s.queries.HasLocalPassword(ctx, userID)
}

func (s *UserPostgresRepository) UpdateLocalPassword(ctx context.Context, userID, oldHash, newHash string) error {
	count, err := s.queries.UpdateLocalPassword(ctx, sqlc.UpdateLocalPasswordParams{UserID: userID, CurrentHash: oldHash, NewPassword: newHash})
	if err != nil {
		return err
	}
	if count == 0 {
		return modelerrors.ErrConflict
	}
	return nil
}

func (s *UserPostgresRepository) CreateLocalSession(ctx context.Context, tokenHash []byte, userID string, expires time.Time) error {
	return s.queries.CreateLocalSession(ctx, sqlc.CreateLocalSessionParams{TokenHash: tokenHash, UserID: userID, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}})
}

func (s *UserPostgresRepository) UserForLocalSession(ctx context.Context, tokenHash []byte) (identitymodel.User, error) {
	row, err := s.queries.UserForLocalSession(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identitymodel.User{}, identity.ErrInvalidCredentials
		}
		return identitymodel.User{}, err
	}
	return identitymodel.User{ID: row.ID, Username: row.Username, Email: row.Email}, nil
}

func (s *UserPostgresRepository) RevokeLocalSession(ctx context.Context, tokenHash []byte) error {
	return s.queries.RevokeLocalSession(ctx, tokenHash)
}

func (s *UserPostgresRepository) LinkKeycloakIdentityByEmail(ctx context.Context, subject, email string) (identitymodel.User, error) {
	row, err := s.queries.LinkKeycloakIdentityByEmail(ctx, sqlc.LinkKeycloakIdentityByEmailParams{Email: email, Subject: pgtype.Text{String: subject, Valid: true}})
	if err != nil {
		return identitymodel.User{}, err
	}
	if row.CandidateCount == 0 {
		return identitymodel.User{}, identity.ErrNoLocalAccountToLink
	}
	if row.CandidateCount != 1 || !row.ID.Valid {
		return identitymodel.User{}, identity.ErrAmbiguousLocalAccount
	}
	return identitymodel.User{ID: row.ID.String, Username: row.Username.String, Email: row.Email.String}, nil
}

func (s *UserPostgresRepository) HasLocalAccountEmail(ctx context.Context, email string) (bool, error) {
	return s.queries.HasLocalAccountEmail(ctx, email)
}
