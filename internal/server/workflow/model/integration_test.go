package model

import (
	"encoding/json"
	"testing"
)

func TestIntegrationRuleValidation(t *testing.T) {
	valid := AutomationRule{ID: "feedback", Enabled: true, Event: IntegrationAction, SourceStageID: "in_review", Action: CallAgent, Integration: &IntegrationTrigger{ConnectionID: "connection", ActionID: "review_added", Parameters: json.RawMessage(`{}`)}, PromptText: "Address the review", SessionPolicy: ContinueOrCreateIfAbsent}
	for _, test := range []struct {
		name   string
		change func(*AutomationRule)
		valid  bool
	}{
		{"call", func(*AutomationRule) {}, true},
		{"move", func(r *AutomationRule) {
			r.Action = MoveTask
			r.TargetStageID = "in_progress"
			r.PromptText = ""
			r.SessionPolicy = ""
		}, true},
		{"close", func(r *AutomationRule) {
			r.Action = MoveTask
			r.TargetStageID = "done"
			r.PromptText = ""
			r.SessionPolicy = ""
		}, false},
		{"no prompt", func(r *AutomationRule) { r.PromptText = " " }, false},
		{"unknown policy", func(r *AutomationRule) { r.SessionPolicy = "auto" }, false},
		{"no integration", func(r *AutomationRule) { r.Integration = nil }, false},
		{"closed source", func(r *AutomationRule) { r.SourceStageID = "done" }, false},
		{"internal with provider", func(r *AutomationRule) { r.Event = RunSucceeded }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := DefaultDefinition("Board")
			d.ID = "board"
			d.Version = 1
			rule := valid
			test.change(&rule)
			d.Rules = append(d.Rules, rule)
			if err := d.Validate(); (err == nil) != test.valid {
				t.Fatalf("validation: %v", err)
			}
		})
	}
	d := DefaultDefinition("Board")
	d.ID = "board"
	d.Rules = append(d.Rules, valid, valid)
	d.Rules[4].ID = "duplicate-match"
	if d.Validate() == nil {
		t.Fatal("duplicate enabled integration match accepted")
	}
	d.Rules[4].Integration = &IntegrationTrigger{ConnectionID: "other", ActionID: "review_added", Parameters: json.RawMessage(`{}`)}
	if err := d.Validate(); err != nil {
		t.Fatal("different connections must be independent", err)
	}
}
