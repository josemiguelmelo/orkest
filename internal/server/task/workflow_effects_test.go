package task

import (
	"reflect"
	"testing"
)

func TestWorkflowPlanningUsesPreassignedIdentity(t *testing.T) {
	input := TaskWorkflowEffects{Command: WorkflowCommand{Action: "start", SessionMode: "new"}, Start: true, StageVersion: 2}
	first, err := PlanWorkflowPersistence("task", "run", input, "", "", "preassigned-command")
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanWorkflowPersistence("task", "run", input, "", "", "preassigned-command")
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("planning changed effects for the same snapshot: %v", err)
	}
	if first.Execution == nil || first.Execution.ID != "preassigned-command" || first.Execution.Options.CommandID != "preassigned-command" || first.History != nil {
		t.Fatalf("preassigned identity or canonical history changed: %+v", first)
	}
}

func TestCompletionWithoutCommandKeepsHistoricalEventIdentity(t *testing.T) {
	effects, err := PlanWorkflowPersistence("task", "run", TaskWorkflowEffects{Command: WorkflowCommand{Action: "complete"}, Completed: true, StageVersion: 2}, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(effects.Events) != 1 {
		t.Fatalf("completion events=%d", len(effects.Events))
	}
	event := effects.Events[0]
	if event.ID != "task:2:completed:" || event.CausationID != "" || event.CorrelationID != "" || event.RunID != "run" {
		t.Fatalf("historical completion identity changed: %+v", event)
	}
}
