package agentruntimeprofile

import (
	"context"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agent"
)

type Profile struct {
	ID               string            `json:"id"`
	WorkspaceID      string            `json:"workspaceId"`
	Name             string            `json:"name"`
	Runtime          agent.Runtime     `json:"runtime"`
	ModelProvider    string            `json:"modelProvider"`
	ModelIdentifier  string            `json:"modelIdentifier"`
	Configuration    map[string]string `json:"-"`
	HasConfiguration bool              `json:"hasConfiguration"`
	CreatedAt        time.Time         `json:"createdAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

type Repository interface {
	List(context.Context, string) ([]Profile, error)
	Get(context.Context, string, string) (Profile, error)
	Save(context.Context, Profile, bool) (Profile, error)
	Delete(context.Context, string, string) error
}
