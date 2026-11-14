package model

import "time"

type Status struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"taskId"`
	RuleID       string    `json:"ruleId"`
	StageVersion int64     `json:"stageVersion"`
	ActionID     string    `json:"actionId"`
	State        string    `json:"state"`
	Error        string    `json:"error"`
	CreatedAt    time.Time `json:"createdAt"`
}

// StatusPosition is a keyset position in the combined observation/effect history.
type StatusPosition struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

type StatusPage struct {
	Items      []Status `json:"items"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

const StatusPageSize = 50
