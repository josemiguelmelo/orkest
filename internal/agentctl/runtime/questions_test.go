package runtime_test

import (
	"encoding/json"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

func TestACPQuestionAnswerAndFollowUp(t *testing.T) {
	controller := newOpenCodeController(t)
	_, events, stop, err := controller.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err := controller.Prompt("question-command", "question"); err != nil {
		t.Fatal(err)
	}
	request := nextEvent(t, events, interaction.EventQuestionRequested)
	if request.Question == nil || request.Question.ToolCallID != "question-tool" {
		t.Fatal("question details lost")
	}
	response := interaction.QuestionResponse{TaskID: "task", RunID: "run", SessionID: request.SessionID, CommandID: request.CommandID, RequestID: request.RequestID, SubmissionID: "answer", Answer: interaction.QuestionAnswer{Action: "accept", Content: json.RawMessage(`{"manager":"pnpm"}`)}}
	if err := controller.RespondQuestion(response); err != nil {
		t.Fatal(err)
	}
	update := nextEvent(t, events, interaction.EventUpdate)
	if update.Update.Text != "Using pnpm" {
		t.Fatal("runtime did not consume answer")
	}
	nextEvent(t, events, interaction.EventPromptCompleted)
	if err := controller.RespondQuestion(response); err != nil {
		t.Fatalf("answer retry after turn completion: %v", err)
	}
	if err := controller.Prompt("follow-up", "Continue"); err != nil {
		t.Fatal(err)
	}
	nextEvent(t, events, interaction.EventPromptCompleted)
}
