package identity

import (
	"context"

	identitymodel "github.com/josemiguelmelo/orkest/internal/server/identity/model"
)

type UserRegistrationRepository interface {
	RegisterUser(context.Context, string, string, string) (identitymodel.User, error)
	IsPlatformAdmin(context.Context, string) (bool, error)
}
