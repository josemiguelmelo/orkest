package coderepository

import (
	"context"

	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
)

type IntegratedCodeRepositoryRepository interface {
	CreateIntegratedCodeRepository(context.Context, coderepositorymodel.CodeRepository) (coderepositorymodel.CodeRepository, error)
}

type CodingIntegrationRepository interface {
	ListCodingIntegrations(context.Context, string) ([]coderepositorymodel.CodingIntegration, error)
	GetCodingIntegration(context.Context, string, string) (coderepositorymodel.CodingIntegration, string, error)
	CreateCodingIntegration(context.Context, coderepositorymodel.CodingIntegration, string) (coderepositorymodel.CodingIntegration, error)
	UpdateCodingIntegration(context.Context, coderepositorymodel.CodingIntegration, string) error
	DeleteCodingIntegration(context.Context, string, string) error
}
