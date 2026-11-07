package runtimehost

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestQuestionCallbacksProtectStringsWithoutTreatingMetadataFlagsAsSecrets(t *testing.T) {
	callbacks := &sessionCallbacks{secrets: []string{"false", "1", "private-provider-key"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	callbacks.attachObserver(ctx, cancel, agentruntime.PromptObserver{Question: func(context.Context, interaction.Question) (interaction.QuestionAnswer, error) {
		calls++
		return interaction.QuestionAnswer{Action: "decline"}, nil
	}})
	question := interaction.Question{Message: "Choose", Schema: json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string","minLength":1,"_meta":{"codex":{"isSecret":false}}}}}`)}
	if _, err := callbacks.Question(ctx, question); err != nil {
		t.Fatal("ordinary numeric/boolean metadata blocked a question", err)
	}
	question.Message = "The value is private-provider-key"
	if _, err := callbacks.Question(ctx, question); err == nil {
		t.Fatal("protected value reached task chat")
	}
	question.Message = "Choose"
	question.Schema = json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string","title":"Use private-provider-key"}}}`)
	if _, err := callbacks.Question(ctx, question); err == nil {
		t.Fatal("protected schema label reached task chat")
	}
	if calls != 1 {
		t.Fatal("protected questions reached the observer")
	}
}
