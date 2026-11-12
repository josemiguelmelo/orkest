package model

import "time"

type Schedule struct {
	Frequency string `json:"frequency"`
	TimeZone  string `json:"timeZone"`
	Hour      int    `json:"hour"`
	Minute    int    `json:"minute"`
	Weekday   int    `json:"weekday"`
}
type Input struct {
	Name               string   `json:"name"`
	Instructions       string   `json:"instructions"`
	AgentID            string   `json:"agentId"`
	RepositoryScope    string   `json:"repositoryScope"`
	RepositoryIDs      []string `json:"repositoryIds"`
	Schedule           Schedule `json:"schedule"`
	Enabled            bool     `json:"enabled"`
	MaxDurationMinutes int      `json:"maxDurationMinutes"`
}
type Job struct {
	Input
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspaceId"`
	Revision     int64     `json:"revision"`
	AuthorizedBy string    `json:"authorizedBy"`
	NextDueAt    time.Time `json:"nextDueAt"`
	CreatedAt    time.Time `json:"createdAt"`
}
type Run struct {
	ID              string    `json:"id"`
	JobID           string    `json:"jobId"`
	WorkspaceID     string    `json:"workspaceId"`
	OrganizationID  string    `json:"-"`
	ActorID         string    `json:"-"`
	Revision        int64     `json:"revision"`
	Input           Input     `json:"input"`
	RepositoryIDs   []string  `json:"repositoryIds"`
	ScheduledFor    time.Time `json:"scheduledFor"`
	State           string    `json:"state"`
	Reason          string    `json:"reason"`
	RunID           string    `json:"runId"`
	CommandID       string    `json:"commandId"`
	CreatedAt       time.Time `json:"createdAt"`
	ClaimToken      string    `json:"-"`
	DeliveryExpired bool      `json:"-"`
	ClaimAttempt    int32     `json:"-"`
	CancelRequested bool      `json:"cancelRequested"`
}
