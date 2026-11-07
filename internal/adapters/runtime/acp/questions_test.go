package acp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

type questionCallbacks struct {
	recordedCallbacks
	question interaction.Question
	answer   interaction.QuestionAnswer
}

func (c *questionCallbacks) Question(_ context.Context, question interaction.Question) (interaction.QuestionAnswer, error) {
	c.question = question
	return c.answer, nil
}

func TestACPQuestionsPreserveNativeSchemaAndSessionScope(t *testing.T) {
	for _, schema := range []string{
		`{"type":"object","properties":{"pm":{"type":"string","oneOf":[{"const":"npm","title":"NPM"}]},"pm_custom":{"type":"string"}},"required":["pm"]}`,
		`{"type":"object","properties":{"question":{"type":"string","enum":["yes","no"],"_meta":{"codex":{"isOther":true,"isSecret":false}}},"question_note":{"type":"string"}},"required":["question"]}`,
		`{"type":"object","properties":{"targets":{"type":"array","items":{"type":"string","enum":["web","server"]},"uniqueItems":true}},"required":["targets"]}`,
	} {
		callbacks := &questionCallbacks{answer: interaction.QuestionAnswer{Action: "accept", Content: json.RawMessage(`{"question":"yes"}`)}}
		client := &client{sessionID: "native", callbacks: callbacks}
		if client.initializeRequest().ClientCapabilities.Elicitation.Form == nil {
			t.Fatal("form support not advertised")
		}
		raw := json.RawMessage(`{"mode":"form","sessionId":"native","toolCallId":"tool","message":"Choose","requestedSchema":` + schema + `}`)
		response, protocolErr := client.handleRequest(context.Background(), "elicitation/create", raw)
		if protocolErr != nil {
			t.Fatal(protocolErr)
		}
		if callbacks.question.ToolCallID != "tool" || string(callbacks.question.Schema) != schema || response.(interaction.QuestionAnswer).Action != "accept" {
			t.Fatalf("lost question mapping: %+v", callbacks.question)
		}
		client.sessionID = "different"
		if _, err := client.handleRequest(context.Background(), "elicitation/create", raw); err == nil {
			t.Fatal("accepted another session's question")
		}
	}
	client := &client{callbacks: &recordedCallbacks{}}
	if client.initializeRequest().ClientCapabilities.Elicitation != nil {
		t.Fatal("advertised questions without a handler")
	}
}
