package identity

import "context"

type UserRegistrationRepository interface {
	RegisterUser(context.Context, string, string, string) (User, error)
	IsPlatformAdmin(context.Context, string) (bool, error)
}
