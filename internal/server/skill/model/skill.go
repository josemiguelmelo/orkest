package model

import (
	"time"
)

type Scope string

const (
	ScopeGlobal       Scope = "global"
	ScopeOrganization Scope = "organization"
	ScopeWorkspace    Scope = "workspace"
)

type Skill struct {
	ID             string    `json:"id"`
	Scope          Scope     `json:"scope"`
	OrganizationID string    `json:"organizationId,omitempty"`
	WorkspaceID    string    `json:"workspaceId,omitempty"`
	SkillMD        string    `json:"skillMD"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Input struct {
	SkillMD string `json:"skillMD"`
}
