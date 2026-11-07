package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

type questionController struct {
	commandController
	response interaction.QuestionResponse
}

func (c *questionController) RespondQuestion(response interaction.QuestionResponse) error {
	c.response = response
	return nil
}

func TestQuestionAnswerHTTPRetainsScopeAndTypedContent(t *testing.T) {
	controller := &questionController{}
	server, err := NewServer(context.Background(), controller)
	if err != nil {
		t.Fatal(err)
	}
	response := request(server, http.MethodPost, "/agent/questions/respond", `{"taskId":"task","taskSessionId":"conversation","runId":"run","sessionId":"native","commandId":"command","requestId":"question","submissionId":"submission","answeredBy":"user","answer":{"action":"accept","content":{"enabled":false,"count":2}}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("response: %d %s", response.Code, response.Body.String())
	}
	value := controller.response
	if value.TaskID != "task" || value.RunID != "run" || value.TaskSessionID != "conversation" || value.SessionID != "native" || value.CommandID != "command" || value.RequestID != "question" || value.SubmissionID != "submission" || value.AnsweredBy != "user" || value.Answer.Action != "accept" || string(value.Answer.Content) != `{"enabled":false,"count":2}` {
		t.Fatalf("lost answer identity/content: %+v", value)
	}
}
