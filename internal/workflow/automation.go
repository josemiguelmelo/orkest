package workflow

import modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"

type EventType string

const (
	StageEntered   EventType = "stage_entered"
	RunSucceeded   EventType = "run_succeeded"
	RunFailed      EventType = "run_failed"
	StageCompleted EventType = "stage_completed"
)

type ActionType string

const (
	MoveTask  ActionType = "move_task"
	StartWork ActionType = "start_work"
)

type AutomationRule struct {
	ID            string     `json:"id"`
	Enabled       bool       `json:"enabled"`
	Event         EventType  `json:"event"`
	SourceStageID string     `json:"sourceStageId"`
	Action        ActionType `json:"action"`
	TargetStageID string     `json:"targetStageId,omitempty"`
}

func validateRules(d Definition) error {
	if len(d.Rules) > 200 {
		return modelerrors.ErrInvalid
	}
	ids, matches := map[string]bool{}, map[string]bool{}
	for _, r := range d.Rules {
		_, source := d.Stage(r.SourceStageID)
		key := string(r.Event) + ":" + r.SourceStageID
		if r.ID == "" || len(r.ID) > 100 || ids[r.ID] || !source || r.SourceStageID == d.ClosedStageID {
			return modelerrors.ErrInvalid
		}
		ids[r.ID] = true
		if r.Enabled && matches[key] {
			return modelerrors.ErrInvalid
		}
		if r.Enabled {
			matches[key] = true
		}
		switch r.Event {
		case StageEntered:
			if r.Action != StartWork || r.TargetStageID != "" {
				return modelerrors.ErrInvalid
			}
		case RunSucceeded, StageCompleted:
			if _, ok := d.Stage(r.TargetStageID); !ok || r.TargetStageID == r.SourceStageID || r.Action != MoveTask {
				return modelerrors.ErrInvalid
			}
		default:
			return modelerrors.ErrInvalid
		}
	}
	return nil
}
