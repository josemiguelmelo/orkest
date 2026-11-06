package workflowautomation

import (
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	definition "github.com/josemiguelmelo/orkest/internal/server/workflow"
)

// TransitionCommand describes stage changes and optional reassignment only.
type TransitionCommand struct {
	Input           TransitionRequest
	AssigneeAgentID *string
}

// TransitionSnapshot contains facts read under the workflow and task locks.
// RecordedCommand lets persistence reconcile a replay before validating new effects.
type TransitionSnapshot struct {
	Workflow           definition.Definition
	Task               task.Task
	ActiveLaunch       bool
	RecordedCommand    bool
	RecordedInput      *task.WorkflowCommand
	ActorID, RunID     string
	CandidateCommandID string
}

// TransitionEffects describes effects to persist atomically. Input is the
// canonical command used for deduplication, including when no effects are needed.
type TransitionEffects struct {
	Input         TransitionRequest
	Persistence   task.WorkflowPersistenceEffects
	Replay        bool
	Plan          StageTransitionPlan
	Moved         bool
	StageVersion  int64
	CompleteStage bool
}

// PlanTaskTransition performs no I/O and must run against the locked snapshot.
func PlanTaskTransition(snapshot TransitionSnapshot, command TransitionCommand) (TransitionEffects, error) {
	current := snapshot.Task
	in := command.Input
	if in.Action != "move" {
		in.StageID = ""
	}
	decision := TransitionEffects{Input: in, StageVersion: current.StageVersion}
	// Replays have no new effects. The repository compares the canonical input
	// with the stored command before returning the existing task.
	if snapshot.RecordedCommand {
		if snapshot.RecordedInput != nil && *snapshot.RecordedInput != task.WorkflowCommand(in) {
			return TransitionEffects{}, modelerrors.ErrConflict
		}
		decision.Replay = true
		return decision, nil
	}
	if in.StageVersion != 0 && in.StageVersion != current.StageVersion {
		return TransitionEffects{}, modelerrors.ErrConflict
	}
	target, assignee := current.Status, current.AssigneeAgentID
	if in.Action == "move" {
		target = in.StageID
	}
	if command.AssigneeAgentID != nil {
		assignee = *command.AssigneeAgentID
	}
	trigger := Enter
	switch in.Action {
	case "start":
		trigger = Start
	case "complete":
		trigger = Complete
	case "move":
	default:
		return TransitionEffects{}, modelerrors.ErrInvalid
	}
	decision.Moved = target != current.Status
	if decision.Moved || in.Action != "move" || assignee != current.AssigneeAgentID {
		if current.StageWorkState == task.WorkStateQueued || current.StageWorkState == task.WorkStateRunning || snapshot.ActiveLaunch {
			return TransitionEffects{}, modelerrors.ErrConflict
		}
	}
	plan := StageTransitionPlan{StageID: target, AssigneeAgentID: assignee}
	if decision.Moved || in.Action == "start" {
		var err error
		plan, err = PlanStageTransition(snapshot.Workflow, current.Status, target, assignee, trigger)
		if err != nil {
			return TransitionEffects{}, err
		}
	}
	if decision.Moved {
		decision.StageVersion++
	}
	decision.CompleteStage = in.Action == "complete"
	plan.Completed = decision.CompleteStage
	decision.Plan = plan
	var err error
	decision.Persistence, err = task.PlanWorkflowPersistence(current.ID, snapshot.RunID, task.TaskWorkflowEffects{Command: task.WorkflowCommand(in), Start: plan.Start, Completed: decision.CompleteStage, Moved: decision.Moved, StageVersion: decision.StageVersion}, in.CommandID, in.CommandID, snapshot.CandidateCommandID)
	return decision, err
}

func (s *WorkflowAutomationService) PlanStageChange(snapshot task.TaskWorkflowSnapshot, command task.WorkflowCommand, assigneeID string) (task.TaskWorkflowEffects, error) {
	effects, err := PlanTaskTransition(TransitionSnapshot{Workflow: snapshot.Workflow, Task: snapshot.Task, ActiveLaunch: snapshot.ActiveLaunch, RecordedCommand: snapshot.RecordedCommand, RecordedInput: snapshot.RecordedInput, ActorID: snapshot.ActorID, RunID: snapshot.RunID, CandidateCommandID: snapshot.CandidateCommandID}, TransitionCommand{Input: TransitionRequest(command), AssigneeAgentID: &assigneeID})
	if err != nil {
		return task.TaskWorkflowEffects{}, err
	}
	return task.TaskWorkflowEffects{Command: task.WorkflowCommand(effects.Input), Persistence: effects.Persistence, Replay: effects.Replay, StageID: effects.Plan.StageID, AssigneeAgentID: effects.Plan.AssigneeAgentID, Start: effects.Plan.Start, Completed: effects.CompleteStage, Moved: effects.Moved, StageVersion: effects.StageVersion}, nil
}
