package coderepository

import "context"

type CodeReposRepository interface {
	CodeRepositories(context.Context, string, string) ([]CodeRepository, error)
	CodeRepository(context.Context, string, string, string) (CodeRepository, error)
	CreateCodeRepository(context.Context, string, string, string, string) (CodeRepository, error)
	DeleteCodeRepository(context.Context, string, string, string) error
}
