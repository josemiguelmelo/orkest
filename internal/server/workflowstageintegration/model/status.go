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
