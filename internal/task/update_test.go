package task_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/workflow"
	"github.com/josemiguelmelo/orkest/internal/workflowautomation"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

func updateSnapshot() task.TaskWorkflowSnapshot {
	d := workflow.DefaultDefinition("Board")
	d.Stages[1].ResponsibleAgentID = "stage-agent"
	return task.TaskWorkflowSnapshot{Workflow: d, Task: task.Task{Status: "todo", StageVersion: 4, AssigneeAgentID: "agent", SourceProvider: "manual", Title: "Original", Description: "Original description", Labels: []string{"original"}, StageWorkState: task.WorkStateIdle}}
}

type updateRepositoryStub struct {
	snapshot task.TaskWorkflowSnapshot
	effects  task.TaskUpdateEffects
}

func (r *updateRepositoryStub) WithTaskUpdate(_ context.Context, _, _, _, _ string, input task.TaskInput, decide func(task.TaskWorkflowSnapshot, task.TaskInput) (task.TaskUpdateEffects, error)) (task.Task, error) {
	var err error
	r.effects, err = decide(r.snapshot, input)
	return task.Task{}, err
}

type updateAccessStub struct{}

func (updateAccessStub) GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error) {
	return workspace.Workspace{Role: "member"}, nil
}

func runTaskUpdate(snapshot task.TaskWorkflowSnapshot, input task.TaskInput) (task.TaskUpdateEffects, error) {
	if input.Title == "" {
		input.Title = "Edited"
	}
	repo := &updateRepositoryStub{snapshot: snapshot}
	planner := workflowautomation.NewWorkflowAutomationService(nil, nil, nil, nil)
	service := task.NewTaskService(nil, nil, repo, planner, updateAccessStub{})
	_, err := service.Update(context.Background(), "actor", "org", "workspace", "task", input)
	return repo.effects, err
}

func TestTaskUpdatePreservesSourceContent(t *testing.T) {
	for _, imported := range []bool{false, true} {
		t.Run(map[bool]string{false: "manual", true: "imported"}[imported], func(t *testing.T) {
			snapshot := updateSnapshot()
			snapshot.Task.StageWorkState = task.WorkStateRunning
			snapshot.ActiveLaunch = true
			if imported {
				snapshot.Task.SourceProvider = "github_cloud"
			}
			input := task.TaskInput{Title: "Edited", Description: "Edited description", Labels: []string{"edited"}, Status: "todo", StageVersion: 4, AssigneeAgentID: "agent", RepositoryIDs: []string{"repo"}}
			before := input
			decision, err := runTaskUpdate(snapshot, input)
			if err != nil || decision.Workflow.Moved || decision.Workflow.Start {
				t.Fatalf("content edit while active: %+v, %v", decision, err)
			}
			want := input
			if imported {
				want.Title, want.Description, want.Labels = snapshot.Task.Title, snapshot.Task.Description, snapshot.Task.Labels
			}
			if !reflect.DeepEqual(decision.Content, want) || !reflect.DeepEqual(input, before) {
				t.Fatalf("content policy: got %+v, want %+v; input %+v", decision.Content, want, input)
			}
		})
	}
}

func TestTaskUpdateMovementTakesPrecedence(t *testing.T) {
	snapshot := updateSnapshot()
	for _, start := range []bool{false, true} {
		input := task.TaskInput{Status: "in_progress", AssigneeAgentID: "agent", StageVersion: 4, StartWork: start, CompleteStage: !start}
		decision, err := runTaskUpdate(snapshot, input)
		if err != nil || decision.Workflow.Command.Action != "move" || !decision.Workflow.Moved || decision.Workflow.Start || decision.Workflow.Completed {
			t.Fatalf("legacy movement: %+v, %v", decision, err)
		}
	}
}
