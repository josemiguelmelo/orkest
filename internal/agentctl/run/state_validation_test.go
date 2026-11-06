package run

import (
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

func TestConversationTransitionValidationSupportsLegacyAndTaskSessions(t *testing.T) {
	for _, taskSession := range []bool{false, true} {
		state := conversationState{usesTaskSession: taskSession}
		events := []interaction.Event{{Type: interaction.EventPromptAccepted, CommandID: "one", Text: "hello"}}
		if taskSession {
			events = append(events, interaction.Event{Type: interaction.EventPromptStarted, CommandID: "one"})
		}
		events = append(events, interaction.Event{Type: interaction.EventPromptCompleted, CommandID: "one"}, interaction.Event{Type: interaction.EventRunCompleted})
		for _, event := range events {
			if err := state.validate(event); err != nil {
				t.Fatalf("valid legacy=%v event=%s rejected", !taskSession, event.Type)
			}
			state.apply(event)
		}
		if err := state.validate(interaction.Event{Type: interaction.EventPromptAccepted, CommandID: "two"}); err == nil {
			t.Fatal("accepted command after run ended")
		}
	}
	state := conversationState{}
	if err := state.validate(interaction.Event{Type: interaction.EventPromptCompleted, CommandID: "unknown"}); err == nil {
		t.Fatal("unknown command completed")
	}
	state.apply(interaction.Event{Type: interaction.EventPromptAccepted, CommandID: "one", Text: "hello"})
	if err := state.validate(interaction.Event{Type: interaction.EventRunCompleted}); err == nil {
		t.Fatal("run ended with queued work")
	}
}
