package organization

import "github.com/josemiguelmelo/orkest/internal/identity"

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type Membership struct {
	User identity.User `json:"user"`
	Role string        `json:"role"`
}
