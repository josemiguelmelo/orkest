package identity

import "context"

type Repository interface {
	RegisterUser(context.Context, string, string, string) (User, error)
	IsPlatformAdmin(context.Context, string) (bool, error)
}
