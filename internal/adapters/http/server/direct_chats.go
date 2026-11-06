package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/agent"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	"github.com/josemiguelmelo/orkest/internal/server/workspace"
)

type directChatAPI struct {
	chats      *orchestration.DirectChatService
	agents     *agent.AgentDefinitionUseCase
	workspaces *workspace.WorkspaceService
}

func (a directChatAPI) routes(r chi.Router) {
	const prefix = "/organizations/{orgID}/workspaces/{workspaceID}/agent-conversations"
	r.Route(prefix, func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.create)
		r.Route("/{conversationID}", func(r chi.Router) {
			r.Delete("/", a.delete)
			r.Get("/runs", a.runs)
			r.Get("/runs/{runID}/events", a.events)
			r.Post("/runs/{runID}/prompts", a.prompt)
			r.Post("/runs/{runID}/cancel", a.cancel)
			r.Post("/runs/{runID}/queue/resume", a.resumeQueue)
			r.Post("/runs/{runID}/queue/discard", a.discardQueue)
			r.Post("/runs/{runID}/permissions/respond", a.respondPermission)
		})
	})
}

func (a directChatAPI) delete(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, _ := scopeParams(r)
	if err := a.authorizeWrite(r, orgID, workspaceID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	respond(w, nil, a.chats.Delete(r.Context(), orgID, workspaceID, chi.URLParam(r, "conversationID")), http.StatusNoContent)
}

func (a directChatAPI) list(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, _ := scopeParams(r)
	if _, err := a.workspaces.GetWorkspace(r.Context(), currentUserID(r), orgID, workspaceID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	items, err := a.chats.List(r.Context(), orgID, workspaceID)
	respond(w, items, err, http.StatusOK)
}

func (a directChatAPI) create(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, _ := scopeParams(r)
	if err := a.authorizeWrite(r, orgID, workspaceID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	var body struct {
		AgentID string `json:"agentId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if _, err := a.agents.Get(r.Context(), currentUserID(r), orgID, workspaceID, body.AgentID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	item, err := a.chats.Create(r.Context(), currentUserID(r), orgID, workspaceID, body.AgentID)
	respond(w, item, err, http.StatusCreated)
}

func (a directChatAPI) runs(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, _ := scopeParams(r)
	if _, err := a.workspaces.GetWorkspace(r.Context(), currentUserID(r), orgID, workspaceID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	if _, err := a.chats.Get(r.Context(), orgID, workspaceID, chi.URLParam(r, "conversationID")); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	items, err := a.chats.Runs(r.Context(), orgID, workspaceID, chi.URLParam(r, "conversationID"))
	views := make([]taskRunView, 0, len(items))
	for _, item := range items {
		views = append(views, runView(item))
	}
	respond(w, views, err, http.StatusOK)
}

func (a directChatAPI) events(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, _ := scopeParams(r)
	if _, err := a.workspaces.GetWorkspace(r.Context(), currentUserID(r), orgID, workspaceID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	after, err := strconv.ParseUint(r.URL.Query().Get("afterSequence"), 10, 64)
	if err != nil && r.URL.Query().Get("afterSequence") != "" {
		writeError(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	events, err := a.chats.Events(r.Context(), orgID, workspaceID, chi.URLParam(r, "conversationID"), chi.URLParam(r, "runID"), after)
	if errors.Is(err, modelerrors.ErrNotFound) || errors.Is(err, orchestration.ErrTaskRunNotFound) {
		writeError(w, http.StatusNotFound, "run_not_found")
		return
	}
	respond(w, events, err, http.StatusOK)
}

func (a directChatAPI) prompt(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, _ := scopeParams(r)
	if err := a.authorizeWrite(r, orgID, workspaceID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	var body promptRequest
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.CommandID) == "" || len(body.CommandID) > 200 || strings.TrimSpace(body.Text) == "" || len(body.Text) > orchestration.MaxPromptBytes {
		writeError(w, http.StatusBadRequest, "invalid_prompt")
		return
	}
	err := a.chats.Prompt(r.Context(), currentUserID(r), orgID, workspaceID, chi.URLParam(r, "conversationID"), chi.URLParam(r, "runID"), body.CommandID, body.Text)
	if errors.Is(err, modelerrors.ErrNotFound) || errors.Is(err, orchestration.ErrTaskRunNotFound) {
		writeError(w, http.StatusNotFound, "conversation_not_found")
		return
	}
	if errors.Is(err, orchestration.ErrCommand) || errors.Is(err, modelerrors.ErrInvalid) {
		writeError(w, http.StatusBadRequest, "invalid_prompt")
		return
	}
	if errors.Is(err, modelerrors.ErrConflict) {
		writeError(w, http.StatusConflict, "prompt_unavailable")
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, "prompt_unavailable")
		return
	}
	respond(w, nil, nil, http.StatusAccepted)
}

func (a directChatAPI) cancel(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, _ := scopeParams(r)
	if err := a.authorizeWrite(r, orgID, workspaceID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	var body struct {
		CommandID string `json:"commandId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	err := a.chats.Cancel(r.Context(), orgID, workspaceID, chi.URLParam(r, "conversationID"), chi.URLParam(r, "runID"), body.CommandID)
	respond(w, nil, err, http.StatusAccepted)
}

func (a directChatAPI) resumeQueue(w http.ResponseWriter, r *http.Request)  { a.queue(w, r, false) }
func (a directChatAPI) discardQueue(w http.ResponseWriter, r *http.Request) { a.queue(w, r, true) }
func (a directChatAPI) queue(w http.ResponseWriter, r *http.Request, discard bool) {
	orgID, workspaceID, _ := scopeParams(r)
	if err := a.authorizeWrite(r, orgID, workspaceID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	err := a.chats.Queue(r.Context(), orgID, workspaceID, chi.URLParam(r, "conversationID"), chi.URLParam(r, "runID"), discard)
	respond(w, nil, err, http.StatusAccepted)
}

func (a directChatAPI) respondPermission(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, _ := scopeParams(r)
	if err := a.authorizeWrite(r, orgID, workspaceID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	var body struct {
		RequestID string `json:"requestId"`
		SessionID string `json:"sessionId"`
		OptionID  string `json:"optionId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	err := a.chats.RespondPermission(r.Context(), orgID, workspaceID, chi.URLParam(r, "conversationID"), chi.URLParam(r, "runID"), orchestration.PermissionResponse{TaskID: chi.URLParam(r, "conversationID"), RunID: chi.URLParam(r, "runID"), RequestID: body.RequestID, SessionID: body.SessionID, OptionID: body.OptionID})
	respond(w, nil, err, http.StatusAccepted)
}

func (a directChatAPI) authorizeWrite(r *http.Request, orgID, workspaceID string) error {
	w, err := a.workspaces.GetWorkspace(r.Context(), currentUserID(r), orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}
