package model

import (
	"time"

	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
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
