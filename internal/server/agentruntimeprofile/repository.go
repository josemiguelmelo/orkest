package agentruntimeprofile

import (
	"context"

	agentruntimeprofilemodel "github.com/josemiguelmelo/orkest/internal/server/agentruntimeprofile/model"
)

type AgentRuntimeProfileRepository interface {
	List(context.Context, string) ([]agentruntimeprofilemodel.Profile, error)
	Get(context.Context, string, string) (agentruntimeprofilemodel.Profile, error)
	Save(context.Context, agentruntimeprofilemodel.Profile, bool) (agentruntimeprofilemodel.Profile, error)
	Delete(context.Context, string, string) error
}
