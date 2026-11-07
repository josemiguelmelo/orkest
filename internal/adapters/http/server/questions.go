package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type questionResponseRequest struct {
	TaskSessionID string                            `json:"taskSessionId"`
	SessionID     string                            `json:"sessionId"`
	CommandID     string                            `json:"commandId"`
	SubmissionID  string                            `json:"submissionId"`
	Answer        orchestrationmodel.QuestionAnswer `json:"answer"`
}

func readQuestionResponse(w http.ResponseWriter, r *http.Request, taskID string) (orchestrationmodel.QuestionResponse, bool) {
	var body questionResponseRequest
	if !decodeBody(w, r, &body) {
		return orchestrationmodel.QuestionResponse{}, false
	}
	if body.SessionID == "" || body.CommandID == "" || body.SubmissionID == "" {
		writeError(w, http.StatusBadRequest, "invalid_question")
		return orchestrationmodel.QuestionResponse{}, false
	}
	return orchestrationmodel.QuestionResponse{AnsweredBy: currentUserID(r), TaskID: taskID, RunID: chi.URLParam(r, "runID"), RequestID: chi.URLParam(r, "questionID"), TaskSessionID: body.TaskSessionID, SessionID: body.SessionID, CommandID: body.CommandID, SubmissionID: body.SubmissionID, Answer: body.Answer}, true
}

func (a taskRunAPI) respondQuestion(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, taskID := scopeParams(r)
	if err := a.tasks.AuthorizeExecution(r.Context(), currentUserID(r), orgID, workspaceID, taskID, true); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	response, ok := readQuestionResponse(w, r, taskID)
	if !ok {
		return
	}
	if err := a.launches.RespondTaskQuestion(r.Context(), orgID, workspaceID, taskID, response.RunID, response); err != nil {
		writeQuestionError(w, err)
		return
	}
	respond(w, map[string]string{"status": "answer_recorded"}, nil, http.StatusOK)
}

func (a directChatAPI) respondQuestion(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, _ := scopeParams(r)
	if err := a.authorizeWrite(r, orgID, workspaceID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	conversationID := chi.URLParam(r, "conversationID")
	response, ok := readQuestionResponse(w, r, conversationID)
	if !ok {
		return
	}
	if err := a.chats.RespondQuestion(r.Context(), orgID, workspaceID, conversationID, response.RunID, response); err != nil {
		writeQuestionError(w, err)
		return
	}
	respond(w, map[string]string{"status": "answer_recorded"}, nil, http.StatusOK)
}

func writeQuestionError(w http.ResponseWriter, err error) {
	var failure *orchestration.AgentctlFailure
	if errors.As(err, &failure) && failure.Code == "invalid_answer" {
		writeError(w, http.StatusBadRequest, "invalid_answer")
		return
	}
	writeError(w, http.StatusConflict, "question_unavailable")
}
