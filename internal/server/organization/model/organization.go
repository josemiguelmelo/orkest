package model

import (
	identitymodel "github.com/josemiguelmelo/orkest/internal/server/identity/model"
)

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type Membership struct {
	User identitymodel.User `json:"user"`
	Role string             `json:"role"`
}
