package workflowautomation

import "github.com/josemiguelmelo/orkest/internal/workflow"

type Event struct{ Type workflow.EventType }

type (
	EventMatcher interface {
		Matches(Event, StageTransitionPlan) bool
	}
	AutomationAction interface {
		Plan(workflow.Definition, StageTransitionPlan) (StageTransitionPlan, error)
	}
	AutomationEvaluator interface {
		Evaluate(workflow.Definition, StageTransitionPlan, Event) (StageTransitionPlan, error)
	}
)

type StageEventMatcher struct {
	Event   workflow.EventType
	StageID string
}

func (m StageEventMatcher) Matches(e Event, p StageTransitionPlan) bool {
	return e.Type == m.Event && p.StageID == m.StageID
}

type MoveTaskAction struct{ TargetStageID string }

func (a MoveTaskAction) Plan(d workflow.Definition, p StageTransitionPlan) (StageTransitionPlan, error) {
	return PlanStageTransition(d, p.StageID, a.TargetStageID, p.AssigneeAgentID, Enter)
}

type StartWorkAction struct{}

func (StartWorkAction) Plan(d workflow.Definition, p StageTransitionPlan) (StageTransitionPlan, error) {
	return PlanStageTransition(d, p.StageID, "", p.AssigneeAgentID, Start)
}

type WorkflowRuleEvaluator struct{}

func (WorkflowRuleEvaluator) Evaluate(d workflow.Definition, p StageTransitionPlan, e Event) (StageTransitionPlan, error) {
	p.Completed = e.Type == workflow.RunSucceeded || e.Type == workflow.StageCompleted
	for _, r := range d.Rules {
		var matcher EventMatcher = StageEventMatcher{Event: r.Event, StageID: r.SourceStageID}
		if !r.Enabled || !matcher.Matches(e, p) {
			continue
		}
		var action AutomationAction = MoveTaskAction{TargetStageID: r.TargetStageID}
		if r.Action == workflow.StartWork {
			action = StartWorkAction{}
		}
		result, err := action.Plan(d, p)
		result.RuleID = r.ID
		return result, err
	}
	return p, nil
}
