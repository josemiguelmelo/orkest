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

// Event is normalized domain data. Runtime identity and delivery metadata live in
// the task event envelope, and are checked before evaluating automation.
type (
	Event          struct{ Type EventType }
	AutomationRule struct {
		ID            string     `json:"id"`
		Enabled       bool       `json:"enabled"`
		Event         EventType  `json:"event"`
		SourceStageID string     `json:"sourceStageId"`
		Action        ActionType `json:"action"`
		TargetStageID string     `json:"targetStageId,omitempty"`
	}
)

type (
	EventMatcher     interface{ Matches(Event, Plan) bool }
	AutomationAction interface {
		Plan(Definition, Plan) (Plan, error)
	}
	AutomationEvaluator interface {
		Evaluate(Definition, Plan, Event) (Plan, error)
	}
)

type StageEventMatcher struct {
	Event   EventType
	StageID string
}

func (m StageEventMatcher) Matches(e Event, p Plan) bool {
	return e.Type == m.Event && p.StageID == m.StageID
}

type MoveTaskAction struct{ TargetStageID string }

func (a MoveTaskAction) Plan(d Definition, p Plan) (Plan, error) {
	return PlanTransition(d, p.StageID, a.TargetStageID, p.AssigneeAgentID, Enter)
}

type StartWorkAction struct{}

func (StartWorkAction) Plan(d Definition, p Plan) (Plan, error) {
	return PlanTransition(d, p.StageID, "", p.AssigneeAgentID, Start)
}

type RuleEvaluator struct{}

func (RuleEvaluator) Evaluate(d Definition, p Plan, e Event) (Plan, error) {
	p.Completed = e.Type == RunSucceeded || e.Type == StageCompleted
	for _, r := range d.Rules {
		var matcher EventMatcher = StageEventMatcher{Event: r.Event, StageID: r.SourceStageID}
		if !r.Enabled || !matcher.Matches(e, p) {
			continue
		}
		var action AutomationAction = MoveTaskAction{TargetStageID: r.TargetStageID}
		if r.Action == StartWork {
			action = StartWorkAction{}
		}
		result, err := action.Plan(d, p)
		result.RuleID = r.ID
		return result, err
	}
	return p, nil
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
