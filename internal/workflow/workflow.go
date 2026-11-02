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
	AutoStart          bool   `json:"autoStart"`
	AutoAdvance        bool   `json:"autoAdvance"`
}

// Definition is ordered; IDs remain stable when names and positions change.
type Definition struct {
	ConfiguredBy string  `json:"configuredBy,omitempty"`
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Version      int64   `json:"version"`
	Stages       []Stage `json:"stages"`
}

func (d Definition) StartStageID() string {
	if len(d.Stages) == 0 {
		return ""
	}
	return d.Stages[0].ID
}

func (d Definition) FinalStageID() string {
	if len(d.Stages) == 0 {
		return ""
	}
	return d.Stages[len(d.Stages)-1].ID
}

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
	final := d.Stages[len(d.Stages)-1]
	if final.AutoStart || final.AutoAdvance {
		return modelerrors.ErrInvalid
	}
	return nil
}

func DefaultDefinition(name string) Definition {
	return Definition{
		Name:    name,
		Version: 1,
		Stages: []Stage{
			{ID: "todo", Name: "Todo", Description: "Ready to pick up"},
			{ID: "in_progress", Name: "In progress", Description: "Work in progress", AutoStart: true, AutoAdvance: true},
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
	StageID         string
	AssigneeAgentID string
	Start           bool
	Completed       bool
}

// PlanTransition has no effects; callers persist its result atomically.
func PlanTransition(d Definition, currentID, targetID, assignee string, trigger Trigger) (Plan, error) {
	current, ok := d.Stage(currentID)
	if !ok {
		return Plan{}, modelerrors.ErrInvalid
	}
	plan := Plan{StageID: currentID, AssigneeAgentID: assignee}
	switch trigger {
	case Succeeded, Complete:
		plan.Completed = true
		if !current.AutoAdvance {
			return plan, nil
		}
		for i, s := range d.Stages {
			if s.ID == currentID {
				if i+1 == len(d.Stages) {
					return plan, nil
				}
				targetID = d.Stages[i+1].ID
				break
			}
		}
	case Start:
		if currentID == d.FinalStageID() || assignee == "" {
			return Plan{}, modelerrors.ErrConflict
		}
		plan.Start = true
		return plan, nil
	case Enter:
	default:
		return Plan{}, modelerrors.ErrInvalid
	}
	next, ok := d.Stage(targetID)
	if !ok {
		return Plan{}, modelerrors.ErrInvalid
	}
	if currentID == d.FinalStageID() && targetID != currentID {
		return Plan{}, modelerrors.ErrConflict
	}
	plan.StageID, plan.Completed = targetID, false
	if next.ResponsibleAgentID != "" {
		plan.AssigneeAgentID = next.ResponsibleAgentID
	}
	plan.Start = next.AutoStart
	if plan.Start && plan.AssigneeAgentID == "" {
		return Plan{}, modelerrors.ErrConflict
	}
	return plan, nil
}
