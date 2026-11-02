// Package workflow defines workspace stages and deterministic transition rules.
package workflow

import (
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type Stage struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	ResponsibleAgentID string `json:"responsibleAgentId"`
}

// Definition is ordered; IDs remain stable when names and positions change.
type Definition struct {
	ConfiguredBy   string           `json:"configuredBy,omitempty"`
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Version        int64            `json:"version"`
	InitialStageID string           `json:"initialStageId"`
	ClosedStageID  string           `json:"closedStageId"`
	Rules          []AutomationRule `json:"rules"`
	Stages         []Stage          `json:"stages"`
}

func (d Definition) StartStageID() string { return d.InitialStageID }
func (d Definition) FinalStageID() string { return d.ClosedStageID }

func (d Definition) Stage(id string) (Stage, bool) {
	for _, s := range d.Stages {
		if s.ID == id {
			return s, true
		}
	}
	return Stage{}, false
}

func (d *Definition) Validate() error {
	d.Name = strings.TrimSpace(d.Name)
	if d.ID == "" || d.Name == "" || len(d.Name) > 200 || len(d.Stages) == 0 || len(d.Stages) > 50 || d.Version < 1 {
		return modelerrors.ErrInvalid
	}
	seen := make(map[string]bool)
	for i := range d.Stages {
		s := &d.Stages[i]
		s.Name = strings.TrimSpace(s.Name)
		s.Description = strings.TrimSpace(s.Description)
		if s.ID == "" || len(s.ID) > 100 || strings.TrimSpace(s.ID) != s.ID || seen[s.ID] || s.Name == "" || len(s.Name) > 200 || len(s.Description) > 2000 {
			return modelerrors.ErrInvalid
		}
		seen[s.ID] = true
	}
	if !seen[d.InitialStageID] || !seen[d.ClosedStageID] {
		return modelerrors.ErrInvalid
	}
	if d.Rules == nil {
		d.Rules = []AutomationRule{}
	}
	return validateRules(*d)
}

func DefaultDefinition(name string) Definition {
	return Definition{
		Name:           name,
		Version:        1,
		InitialStageID: "todo", ClosedStageID: "done",
		Rules: []AutomationRule{
			{ID: "start-progress", Enabled: true, Event: StageEntered, SourceStageID: "in_progress", Action: StartWork},
			{ID: "review-success", Enabled: true, Event: RunSucceeded, SourceStageID: "in_progress", Action: MoveTask, TargetStageID: "in_review"},
			{ID: "review-complete", Enabled: true, Event: StageCompleted, SourceStageID: "in_progress", Action: MoveTask, TargetStageID: "in_review"},
		},
		Stages: []Stage{
			{ID: "todo", Name: "Todo", Description: "Ready to pick up"},
			{ID: "in_progress", Name: "In progress", Description: "Work in progress"},
			{ID: "in_review", Name: "In review", Description: "Ready for review"},
			{ID: "done", Name: "Done", Description: "Completed work"},
		},
	}
}

type Trigger string

const (
	Enter     Trigger = "enter"
	Start     Trigger = "start"
	Succeeded Trigger = "succeeded"
	Complete  Trigger = "complete"
)

type Plan struct {
	RuleID          string
	StageID         string
	AssigneeAgentID string
	Start           bool
	Completed       bool
}

// PlanTransition has no effects; callers persist its result atomically.
func PlanTransition(d Definition, currentID, targetID, assignee string, trigger Trigger) (Plan, error) {
	if _, ok := d.Stage(currentID); !ok {
		return Plan{}, modelerrors.ErrInvalid
	}
	plan := Plan{StageID: currentID, AssigneeAgentID: assignee}
	switch trigger {
	case Succeeded, Complete:
		event := RunSucceeded
		if trigger == Complete {
			event = StageCompleted
		}
		return (RuleEvaluator{}).Evaluate(d, plan, Event{Type: event})
	case Start:
		if currentID == d.ClosedStageID || assignee == "" {
			return Plan{}, modelerrors.ErrConflict
		}
		plan.Start = true
		return plan, nil
	case Enter:
	default:
		return Plan{}, modelerrors.ErrInvalid
	}
	if currentID == d.ClosedStageID && targetID != currentID {
		return Plan{}, modelerrors.ErrConflict
	}
	next, ok := d.Stage(targetID)
	if !ok {
		return Plan{}, modelerrors.ErrInvalid
	}
	plan.StageID = targetID
	if next.ResponsibleAgentID != "" {
		plan.AssigneeAgentID = next.ResponsibleAgentID
	}
	return plan, nil
}
