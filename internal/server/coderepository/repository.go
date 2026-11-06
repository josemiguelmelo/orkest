package coderepository

import (
	"context"

	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
)

type CodeReposRepository interface {
	CodeRepositories(context.Context, string, string) ([]coderepositorymodel.CodeRepository, error)
	CodeRepository(context.Context, string, string, string) (coderepositorymodel.CodeRepository, error)
	DeleteCodeRepository(context.Context, string, string, string) error
}
