package identity

import (
	"context"
	"errors"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	identitymodel "github.com/josemiguelmelo/orkest/internal/server/identity/model"
)

type UserRegistrationService struct{ repository UserRegistrationRepository }

func NewUserRegistrationService(repository UserRegistrationRepository) *UserRegistrationService {
	return &UserRegistrationService{repository: repository}
}

func (s *UserRegistrationService) EnsureUserProfile(ctx context.Context, subject, username, email string) (identitymodel.User, error) {
	return s.repository.RegisterUser(ctx, subject, username, email)
}

func (s *UserRegistrationService) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return s.repository.IsPlatformAdmin(ctx, userID)
}

func (s *UserRegistrationService) PromotePlatformAdmin(ctx context.Context, actorID, userID string) error {
	allowed, err := s.repository.IsPlatformAdmin(ctx, actorID)
	if err != nil {
		return err
	}
	if !allowed {
		return modelerrors.ErrForbidden
	}
	manager, ok := s.repository.(PlatformAdminManagementRepository)
	if !ok {
		return errors.New("platform administrator management is unavailable")
	}
	count, err := manager.PromotePlatformAdmin(ctx, userID)
	if err != nil {
		return err
	}
	if count == 0 {
		return modelerrors.ErrNotFound
	}
	return nil
}
