package model

import (
	"time"
)

type Comment struct {
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspaceId"`
	TaskID       string    `json:"taskId"`
	AuthorUserID string    `json:"authorUserId"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"createdAt"`
}
