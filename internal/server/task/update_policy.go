package task

import (
	"context"
	"fmt"

	"github.com/josemiguelmelo/orkest/internal/platform/ids"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
)

// WorkflowCommand is the canonical identity payload for task workflow mutations.
// Its JSON shape is retained for reconciliation of persisted commands.
type WorkflowCommand struct {
	StageID      string `json:"stageId"`
	StageVersion int64  `json:"stageVersion"`
	Action       string `json:"action"`
	CommandID    string `json:"commandId"`
	SessionMode  string `json:"sessionMode"`
	PromptText   string `json:"promptText"`
}

type TaskWorkflowSnapshot struct {
	Workflow           workflow.Definition
	Task               Task
	ActiveLaunch       bool
	RecordedCommand    bool
	RecordedInput      *WorkflowCommand
	ActorID, RunID     string
	CandidateCommandID string
}

type TaskWorkflowEffects struct {
	Command                  WorkflowCommand
	Persistence              WorkflowPersistenceEffects
	Replay                   bool
	StageID, AssigneeAgentID string
	Start, Completed, Moved  bool
	StageVersion             int64
}

type TaskUpdateEffects struct {
	Content  TaskInput
	Workflow TaskWorkflowEffects
}

// TaskWorkflowPlanner owns movement and execution rules. Task CRUD depends on
// this task-owned interface, with its implementation injected by app wiring.
type TaskWorkflowPlanner interface {
	PlanStageChange(TaskWorkflowSnapshot, WorkflowCommand, string) (TaskWorkflowEffects, error)
}

type TaskUpdateRepository interface {
	WithTaskUpdate(context.Context, string, string, string, string, TaskInput, func(TaskWorkflowSnapshot, TaskInput) (TaskUpdateEffects, error)) (Task, error)
}

func (s *TaskService) Update(ctx context.Context, userID, orgID, workspaceID, taskID string, input TaskInput) (Task, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Task{}, err
	}
	if err := NormalizeTaskContent(&input); err != nil {
		return Task{}, err
	}
	if err := ValidateWorkOptions(input.SessionMode, input.PromptText, input.CommandID); err != nil {
		return Task{}, err
	}
	if input.Status == "" {
		return Task{}, modelerrors.ErrInvalid
	}
	candidateID := ""
	if input.StartWork && input.CommandID == "" {
		var err error
		candidateID, err = ids.NewUUID()
		if err != nil {
			return Task{}, fmt.Errorf("generate task execution command ID: %w", err)
		}
	}
	return s.updates.WithTaskUpdate(ctx, userID, orgID, workspaceID, taskID, input, func(snapshot TaskWorkflowSnapshot, input TaskInput) (TaskUpdateEffects, error) {
		snapshot.CandidateCommandID = candidateID
		return s.planTaskUpdate(snapshot, input)
	})
}

func (s *TaskService) planTaskUpdate(snapshot TaskWorkflowSnapshot, input TaskInput) (TaskUpdateEffects, error) {
	command := WorkflowCommand{Action: "move", StageID: input.Status, StageVersion: input.StageVersion, CommandID: input.CommandID, SessionMode: input.SessionMode, PromptText: input.PromptText}
	if input.StartWork {
		command.Action = "start"
	} else if input.CompleteStage {
		command.Action = "complete"
	}
	if input.Status != snapshot.Task.Status {
		command.Action = "move"
	}
	if command.Action != "move" {
		command.StageID = ""
	}
	effects, err := s.workflow.PlanStageChange(snapshot, command, input.AssigneeAgentID)
	if err != nil {
		return TaskUpdateEffects{}, err
	}
	return TaskUpdateEffects{Content: PreserveSourceContent(snapshot.Task, input), Workflow: effects}, nil
}
