package model

import (
	"time"

	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
)

type Profile struct {
	ID               string             `json:"id"`
	WorkspaceID      string             `json:"workspaceId"`
	Name             string             `json:"name"`
	Runtime          agentmodel.Runtime `json:"runtime"`
	ModelProvider    string             `json:"modelProvider"`
	ModelIdentifier  string             `json:"modelIdentifier"`
	Configuration    map[string]string  `json:"-"`
	HasConfiguration bool               `json:"hasConfiguration"`
	CreatedAt        time.Time          `json:"createdAt"`
	UpdatedAt        time.Time          `json:"updatedAt"`
}
