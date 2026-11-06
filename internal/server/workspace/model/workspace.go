package model

import (
	identitymodel "github.com/josemiguelmelo/orkest/internal/server/identity/model"
)

type Workspace struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	Role           string `json:"role,omitempty"`
}

type Membership struct {
	User identitymodel.User `json:"user"`
	Role string             `json:"role"`
}
