package executionprofile

import (
	"context"
	"time"

	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/server/workspace"
)

type Input struct {
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	Runtime        coreruntime.Runtime `json:"runtime"`
	ToolchainImage string              `json:"toolchainImage"`
}

type Profile struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Input
	Generation     int64     `json:"generation"`
	Status         string    `json:"status"`
	ExecutionImage string    `json:"executionImage"`
	BuildError     string    `json:"buildError"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Build struct {
	RuntimeVersion    string              `json:"runtimeVersion"`
	ResolvedToolchain string              `json:"resolvedToolchain"`
	ResolvedBase      string              `json:"resolvedBase"`
	ID                string              `json:"id"`
	ProfileID         string              `json:"profileId"`
	WorkspaceID       string              `json:"workspaceId"`
	Generation        int64               `json:"generation"`
	Runtime           coreruntime.Runtime `json:"runtime"`
	ToolchainImage    string              `json:"toolchainImage"`
	BaseImage         string              `json:"baseImage"`
	Destination       string              `json:"destination"`
	Status            string              `json:"status"`
	ExecutionImage    string              `json:"executionImage"`
	BuildError        string              `json:"buildError"`
	Owner             string              `json:"-"`
	CreatedAt         time.Time           `json:"createdAt"`
}

type ExecutionProfileRepository interface {
	List(context.Context, string) ([]Profile, error)
	Get(context.Context, string, string) (Profile, error)
	Save(context.Context, Profile, Build, bool) (Profile, error)
	Builds(context.Context, string, string) ([]Build, error)
	Claim(context.Context, string) (Build, bool, error)
	Heartbeat(context.Context, string, string) error
	PinInputs(context.Context, Build) error
	Finish(context.Context, Build, string, string, string) error
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}

// Builder owns external image effects. Reconcile never starts another build.
type Builder interface {
	ResolveInputs(context.Context, Build) (Build, error)
	BuildAndPush(context.Context, Build) (string, error)
	Reconcile(context.Context, Build) (string, error)
}
