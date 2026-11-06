package executionprofile

import (
	"context"

	executionprofilemodel "github.com/josemiguelmelo/orkest/internal/server/executionprofile/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type ExecutionProfileRepository interface {
	List(context.Context, string) ([]executionprofilemodel.Profile, error)
	Get(context.Context, string, string) (executionprofilemodel.Profile, error)
	Save(context.Context, executionprofilemodel.Profile, executionprofilemodel.Build, bool) (executionprofilemodel.Profile, error)
	Builds(context.Context, string, string) ([]executionprofilemodel.Build, error)
	Claim(context.Context, string) (executionprofilemodel.Build, bool, error)
	Heartbeat(context.Context, string, string) error
	PinInputs(context.Context, executionprofilemodel.Build) error
	Finish(context.Context, executionprofilemodel.Build, string, string, string) error
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

// Builder owns external image effects. Reconcile never starts another build.
type Builder interface {
	ResolveInputs(context.Context, executionprofilemodel.Build) (executionprofilemodel.Build, error)
	BuildAndPush(context.Context, executionprofilemodel.Build) (string, error)
	Reconcile(context.Context, executionprofilemodel.Build) (string, error)
}
