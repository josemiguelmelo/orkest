package identity

import "context"

type UserRegistrationService struct{ repository Repository }

func NewUserRegistrationService(repository Repository) *UserRegistrationService {
	return &UserRegistrationService{repository: repository}
}

func (s *UserRegistrationService) EnsureUserProfile(ctx context.Context, subject, username, email string) (User, error) {
	return s.repository.RegisterUser(ctx, subject, username, email)
}
