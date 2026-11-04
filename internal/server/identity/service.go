package identity

import "context"

type UserRegistrationService struct{ repository UserRegistrationRepository }

func NewUserRegistrationService(repository UserRegistrationRepository) *UserRegistrationService {
	return &UserRegistrationService{repository: repository}
}

func (s *UserRegistrationService) EnsureUserProfile(ctx context.Context, subject, username, email string) (User, error) {
	return s.repository.RegisterUser(ctx, subject, username, email)
}

func (s *UserRegistrationService) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return s.repository.IsPlatformAdmin(ctx, userID)
}
