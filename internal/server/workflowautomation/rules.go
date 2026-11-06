package workflowautomation

import (
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

type Event struct{ Type workflowmodel.EventType }

type AutomationEvaluator interface {
	Evaluate(workflowmodel.Definition, StageTransitionPlan, Event) (StageTransitionPlan, error)
}

type WorkflowRuleEvaluator struct{}

func (WorkflowRuleEvaluator) Evaluate(definition workflowmodel.Definition, plan StageTransitionPlan, event Event) (StageTransitionPlan, error) {
	plan.Completed = event.Type == workflowmodel.RunSucceeded || event.Type == workflowmodel.StageCompleted
	for _, rule := range definition.Rules {
		if !rule.Enabled || rule.Event != event.Type || rule.SourceStageID != plan.StageID {
			continue
		}
		trigger, targetID := Enter, rule.TargetStageID
		if rule.Action == workflowmodel.StartWork {
			trigger, targetID = Start, ""
		}
		result, err := PlanStageTransition(definition, plan.StageID, targetID, plan.AssigneeAgentID, trigger)
		result.RuleID = rule.ID
		return result, err
	}
	return plan, nil
}
