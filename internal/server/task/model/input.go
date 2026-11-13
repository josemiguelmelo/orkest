package model

import (
	"errors"
	"fmt"

	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

var ErrIntegrationSession = errors.New("integration callback cannot continue this conversation; preserve saved state")

// StartFailure preserves the failing task-start phase while keeping error text
// out of ordinary logs. Cause remains available to classify with errors.Is/As.
type StartFailure struct {
	Stage string
	Cause error
}

func (e *StartFailure) Error() string { return fmt.Sprintf("task start failed at %s", e.Stage) }

func (e *StartFailure) Unwrap() error { return e.Cause }

type TaskInput struct {
	StageVersion    int64    `json:"stageVersion,omitempty"`
	WorkflowID      string   `json:"workflowId"`
	CompleteStage   bool     `json:"completeStage,omitempty"`
	ActorID         string   `json:"-"`
	StartWork       bool     `json:"startWork,omitempty"`
	SessionMode     string   `json:"sessionMode,omitempty"`
	PromptText      string   `json:"promptText,omitempty"`
	CommandID       string   `json:"commandId,omitempty"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Status          string   `json:"status"`
	AssigneeAgentID string   `json:"assigneeAgentId"`
	Labels          []string `json:"labels"`
	RepositoryIDs   []string `json:"repositoryIds"`
}

// StartCommand is an explicit execution request, separate from the task entity.
type StartCommand struct {
	Integration                        *workflowmodel.IntegrationEvent
	Task                               Task
	CommandID, SessionMode, PromptText string
	DeliveryToken                      string
	ExpectedAssigneeID                 string
	GuardAssignment                    bool
}
