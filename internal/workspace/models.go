package workspace

import "github.com/josemiguelmelo/orkest/internal/identity"

type Workspace struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	Role           string `json:"role,omitempty"`
}

type Membership struct {
	User identity.User `json:"user"`
	Role string        `json:"role"`
}
