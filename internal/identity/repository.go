package identity

import "context"

type Repository interface {
	RegisterUser(context.Context, string, string, string) (User, error)
}
