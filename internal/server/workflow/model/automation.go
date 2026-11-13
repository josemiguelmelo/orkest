package model

import (
	"encoding/json"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
)

type EventType string

const (
	StageEntered      EventType = "stage_entered"
	RunSucceeded      EventType = "run_succeeded"
	RunFailed         EventType = "run_failed"
	StageCompleted    EventType = "stage_completed"
	IntegrationAction EventType = "integration_action"
)

type ActionType string

const (
	MoveTask  ActionType = "move_task"
	StartWork ActionType = "start_work"
	CallAgent ActionType = "call_agent"
)

const ContinueOrCreateIfAbsent = "continue_or_create_if_absent"

// IntegrationTrigger selects a provider action without coupling workflow policy to a provider.
type IntegrationTrigger struct {
	ConnectionID string          `json:"connectionId"`
	ActionID     string          `json:"actionId"`
	Parameters   json.RawMessage `json:"parameters"`
}

func validateIntegrationRule(d Definition, r AutomationRule) error {
	if r.Integration == nil || r.Integration.ConnectionID == "" || len(r.Integration.ConnectionID) > 100 || r.Integration.ActionID == "" || len(r.Integration.ActionID) > 100 || !json.Valid(r.Integration.Parameters) || len(r.Integration.Parameters) > 16000 {
		return modelerrors.ErrInvalid
	}
	switch r.Action {
	case MoveTask:
		if _, ok := d.Stage(r.TargetStageID); !ok || r.PromptText != "" || r.SessionPolicy != "" {
			return modelerrors.ErrInvalid
		}
	case CallAgent:
		if r.TargetStageID != "" || strings.TrimSpace(r.PromptText) == "" || len(r.PromptText) > MaxCustomPromptBytes || (r.SessionPolicy != ContinueOrCreateIfAbsent && r.SessionPolicy != "new") {
			return modelerrors.ErrInvalid
		}
	default:
		return modelerrors.ErrInvalid
	}
	return nil
}

// IntegrationEvent identifies a matched observation and preserves external context as data.
type IntegrationEvent struct {
	RuleID               string `json:"ruleId"`
	ConnectionID         string `json:"connectionId"`
	ConfigurationVersion int64  `json:"configurationVersion"`
	ConnectionVersion    int64  `json:"connectionVersion"`
	PromptContext        string `json:"promptContext,omitempty"`
}

type AutomationRule struct {
	ID            string              `json:"id"`
	Enabled       bool                `json:"enabled"`
	Event         EventType           `json:"event"`
	SourceStageID string              `json:"sourceStageId"`
	Action        ActionType          `json:"action"`
	TargetStageID string              `json:"targetStageId,omitempty"`
	Integration   *IntegrationTrigger `json:"integration,omitempty"`
	PromptText    string              `json:"promptText,omitempty"`
	SessionPolicy string              `json:"sessionPolicy,omitempty"`
}

func validateRules(d Definition) error {
	if len(d.Rules) > 200 {
		return modelerrors.ErrInvalid
	}
	ids, matches := map[string]bool{}, map[string]bool{}
	for _, r := range d.Rules {
		_, source := d.Stage(r.SourceStageID)
		key := string(r.Event) + ":" + r.SourceStageID
		if r.Event == IntegrationAction && r.Integration != nil {
			key += ":" + r.Integration.ConnectionID + ":" + r.Integration.ActionID
		}
		if r.ID == "" || len(r.ID) > 100 || ids[r.ID] || !source || r.SourceStageID == d.ClosedStageID {
			return modelerrors.ErrInvalid
		}
		ids[r.ID] = true
		if r.Event != IntegrationAction && (r.Integration != nil || r.PromptText != "" || r.SessionPolicy != "") {
			return modelerrors.ErrInvalid
		}
		if r.Enabled && matches[key] {
			return modelerrors.ErrInvalid
		}
		if r.Enabled {
			matches[key] = true
		}
		switch r.Event {
		case IntegrationAction:
			if err := validateIntegrationRule(d, r); err != nil {
				return err
			}
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
