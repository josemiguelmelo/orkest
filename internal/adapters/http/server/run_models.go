package httpapi

import (
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type taskRunView struct {
	ExecutionImage  string                            `json:"executionImage"`
	RecoveryBlocked bool                              `json:"recoveryBlocked"`
	ExecutionState  orchestrationmodel.ExecutionState `json:"executionState,omitempty"`
	QueuePaused     bool                              `json:"queuePaused"`
	QueueDepth      int                               `json:"queueDepth"`
	RunID           string                            `json:"runId"`
	TaskSessionID   string                            `json:"taskSessionId,omitempty"`
	TaskID          string                            `json:"taskId"`
	Status          orchestrationmodel.LaunchStatus   `json:"status"`
	FailureStage    string                            `json:"failureStage,omitempty"`
	FailureReason   string                            `json:"failureReason,omitempty"`
	LastSequence    uint64                            `json:"lastSequence"`
	UpdatedAt       string                            `json:"updatedAt"`
}

type promptRequest struct {
	CommandID string `json:"commandId"`
	Text      string `json:"text"`
}

type cancelRequest struct {
	CommandID string `json:"commandId"`
}

type permissionResponseRequest struct {
	RequestID string `json:"requestId"`
	SessionID string `json:"sessionId"`
	OptionID  string `json:"optionId"`
}

func runView(item orchestrationmodel.Launch) taskRunView {
	return taskRunView{ExecutionImage: item.ExecutionImage, RunID: item.RunID, TaskSessionID: item.TaskSessionID, ExecutionState: item.ExecutionState, RecoveryBlocked: item.RecoveryBlocked, QueuePaused: item.QueuePaused, QueueDepth: item.QueueDepth, TaskID: item.TaskID, Status: item.Status, FailureStage: item.FailureStage, FailureReason: item.FailureReason, LastSequence: item.LastSequence, UpdatedAt: item.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")}
}

func runViews(items []orchestrationmodel.Launch) []taskRunView {
	views := make([]taskRunView, 0, len(items))
	for _, item := range items {
		views = append(views, runView(item))
	}
	return views
}
