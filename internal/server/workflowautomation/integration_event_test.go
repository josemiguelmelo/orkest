package workflowautomation

import (
	"encoding/json"
	"strings"
	"testing"

	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

func integrationSnapshot() AutomationEventSnapshot {
	d := workflowmodel.DefaultDefinition("Board")
	d.Version = 7
	d.ConfiguredBy = "admin"
	d.Rules = append(d.Rules, workflowmodel.AutomationRule{ID: "feedback", Enabled: true, Event: workflowmodel.IntegrationAction, SourceStageID: "in_review", Action: workflowmodel.CallAgent, Integration: &workflowmodel.IntegrationTrigger{ConnectionID: "connection", ActionID: "review_added", Parameters: json.RawMessage(`{}`)}, PromptText: "Address the review", SessionPolicy: workflowmodel.ContinueOrCreateIfAbsent})
	return AutomationEventSnapshot{Workflow: d, StageID: "in_review", StageVersion: 3, AssigneeAgentID: "agent", WorkState: taskmodel.WorkStateCompleted, IntegrationAuthorized: true, IntegrationConfigurationValid: true, Event: AutomationEvent{ID: "external-review-42", ActorID: "admin", TaskID: "task", Type: workflowmodel.IntegrationAction, StageVersion: 3, Integration: &workflowmodel.IntegrationEvent{RuleID: "feedback", ConnectionID: "connection", ConfigurationVersion: 7, PromptContext: `{"reviewUrl":"https://github.com/acme/repo/pull/42#review-7"}`}}}
}

func TestIntegrationCallbackProducesGuardedPromptCommand(t *testing.T) {
	s := &WorkflowAutomationService{evaluator: WorkflowRuleEvaluator{}}
	effects, err := s.evaluateEvent(integrationSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	execution := effects.Persistence.Execution
	if execution == nil || effects.Moved || effects.Plan.StageID != "in_review" {
		t.Fatalf("unexpected callback: %+v", effects)
	}
	if execution.ID != "external-review-42:start" || execution.Options.ExpectedAssigneeID != "agent" || !execution.Options.GuardAssignment || execution.Options.SessionMode != workflowmodel.ContinueOrCreateIfAbsent || execution.Options.StageVersion != 3 || execution.Options.Integration == nil || execution.Options.Integration.RuleID != "feedback" || !strings.Contains(execution.Options.PromptText, "Address the review") || !strings.Contains(execution.Options.PromptText, "reviewUrl") {
		t.Fatalf("lost command identity/prompt/session guard: %+v", execution)
	}
}

func TestIntegrationCallbackRejectsStaleAndConflictingEvents(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*AutomationEventSnapshot)
		reason string
	}{
		{"old stage", func(s *AutomationEventSnapshot) { s.StageVersion++ }, "stage_changed"},
		{"old configuration", func(s *AutomationEventSnapshot) { s.Workflow.Version++ }, "configuration_changed"},
		{"disabled binding", func(s *AutomationEventSnapshot) { s.Workflow.Rules[3].Enabled = false }, "configuration_changed"},
		{"disabled connection", func(s *AutomationEventSnapshot) { s.IntegrationConfigurationValid = false }, "configuration_changed"},
		{"revoked access", func(s *AutomationEventSnapshot) { s.IntegrationAuthorized = false }, "authorization_revoked"},
		{"active launch", func(s *AutomationEventSnapshot) { s.ActiveLaunch = true }, "busy"},
		{"queued", func(s *AutomationEventSnapshot) { s.WorkState = taskmodel.WorkStateQueued }, "busy"},
		{"running", func(s *AutomationEventSnapshot) { s.WorkState = taskmodel.WorkStateRunning }, "busy"},
		{"unassigned", func(s *AutomationEventSnapshot) { s.AssigneeAgentID = "" }, "callback_prerequisites"},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := integrationSnapshot()
			test.change(&snapshot)
			effects, err := (&WorkflowAutomationService{evaluator: WorkflowRuleEvaluator{}}).evaluateEvent(snapshot)
			if err != nil || effects.Plan != nil || effects.Persistence.Execution != nil || effects.WorkState != "" || effects.FailureReason != test.reason {
				t.Fatalf("unsafe callback: %+v, %v", effects, err)
			}
		})
	}
}

func TestIntegrationMovementUsesExistingStageAssignment(t *testing.T) {
	snapshot := integrationSnapshot()
	rule := &snapshot.Workflow.Rules[3]
	rule.Action = workflowmodel.MoveTask
	rule.TargetStageID = "in_progress"
	rule.PromptText = ""
	rule.SessionPolicy = ""
	snapshot.Workflow.Stages[1].ResponsibleAgentID = "implementer"
	effects, err := (&WorkflowAutomationService{evaluator: WorkflowRuleEvaluator{}}).evaluateEvent(snapshot)
	if err != nil || !effects.Moved || effects.Plan.AssigneeAgentID != "implementer" || effects.Persistence.Execution != nil || len(effects.Persistence.Events) != 1 || effects.Persistence.Events[0].Type != workflowmodel.StageEntered {
		t.Fatalf("incorrect movement: %+v %v", effects, err)
	}
}

func TestIntegrationMovementCanTargetClosedStage(t *testing.T) {
	snapshot := integrationSnapshot()
	rule := &snapshot.Workflow.Rules[3]
	rule.Action = workflowmodel.MoveTask
	rule.TargetStageID = snapshot.Workflow.ClosedStageID
	rule.PromptText = ""
	rule.SessionPolicy = ""
	effects, err := (&WorkflowAutomationService{evaluator: WorkflowRuleEvaluator{}}).evaluateEvent(snapshot)
	if err != nil || !effects.Moved || effects.Plan.StageID != snapshot.Workflow.ClosedStageID {
		t.Fatalf("closed-stage callback move failed: %+v %v", effects, err)
	}
}

func TestIntegrationMovementToSourceStageIsAcknowledgedWithoutMoving(t *testing.T) {
	snapshot := integrationSnapshot()
	rule := &snapshot.Workflow.Rules[3]
	rule.Action = workflowmodel.MoveTask
	rule.TargetStageID = snapshot.StageID
	rule.PromptText = ""
	rule.SessionPolicy = ""
	effects, err := (&WorkflowAutomationService{evaluator: WorkflowRuleEvaluator{}}).evaluateEvent(snapshot)
	if err != nil || effects.Moved || effects.Plan.StageID != snapshot.StageID {
		t.Fatalf("same-stage callback should be a no-op: %+v %v", effects, err)
	}
}
