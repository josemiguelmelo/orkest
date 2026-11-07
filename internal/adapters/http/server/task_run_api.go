package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	"github.com/josemiguelmelo/orkest/internal/server/task"
)

type taskRunAPI struct {
	tasks    *task.TaskService
	launches *orchestration.LaunchService
}

func (a taskRunAPI) routes(r chi.Router) {
	const prefix = "/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/runs"
	r.Route(prefix, func(r chi.Router) {
		r.Get("/", a.list)
		r.Get("/{runID}/events", a.events)
		r.Post("/{runID}/prompts", a.prompt)
		r.Post("/{runID}/cancel", a.cancel)
		r.Post("/{runID}/queue/resume", a.resumeQueue)
		r.Post("/{runID}/queue/discard", a.discardQueue)
		r.Post("/{runID}/permissions/respond", a.respondPermission)
		r.Post("/{runID}/questions/{questionID}/respond", a.respondQuestion)
	})
}

func (a taskRunAPI) list(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, taskID := scopeParams(r)
	if _, err := a.tasks.Get(r.Context(), currentUserID(r), orgID, workspaceID, taskID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	items, err := a.launches.TaskLaunches(r.Context(), orgID, workspaceID, taskID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "run_history_unavailable")
		return
	}
	views := runViews(items)
	respond(w, views, nil, http.StatusOK)
}

func (a taskRunAPI) events(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, taskID := scopeParams(r)
	if _, err := a.tasks.Get(r.Context(), currentUserID(r), orgID, workspaceID, taskID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	after, err := parseEventCursor(r.URL.Query().Get("afterSequence"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	events, err := a.launches.TaskEvents(r.Context(), orgID, workspaceID, taskID, chi.URLParam(r, "runID"), after)
	if err != nil {
		if errors.Is(err, orchestration.ErrTaskRunNotFound) {
			writeError(w, http.StatusNotFound, "run_not_found")
			return
		}
		writeError(w, http.StatusServiceUnavailable, "run_activity_unavailable")
		return
	}
	respond(w, events, nil, http.StatusOK)
}

func (a taskRunAPI) prompt(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, taskID := scopeParams(r)
	if err := a.tasks.AuthorizeExecution(r.Context(), currentUserID(r), orgID, workspaceID, taskID, true); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	var body promptRequest
	if !decodeBody(w, r, &body) {
		return
	}
	body.Text = strings.TrimSpace(body.Text)
	if body.Text == "" || len([]byte(body.Text)) > orchestrationmodel.MaxPromptBytes || strings.TrimSpace(body.CommandID) == "" || len(body.CommandID) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_prompt")
		return
	}
	if err := a.launches.SendTaskPrompt(r.Context(), orgID, workspaceID, taskID, chi.URLParam(r, "runID"), body.CommandID, body.Text); err != nil {
		writeError(w, http.StatusConflict, "prompt_unavailable")
		return
	}
	respond(w, map[string]string{"commandId": body.CommandID}, nil, http.StatusAccepted)
}

func (a taskRunAPI) cancel(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, taskID := scopeParams(r)
	if err := a.tasks.AuthorizeExecution(r.Context(), currentUserID(r), orgID, workspaceID, taskID, true); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	var body cancelRequest
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.CommandID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_command")
		return
	}
	if err := a.launches.CancelTaskPrompt(r.Context(), orgID, workspaceID, taskID, chi.URLParam(r, "runID"), body.CommandID); err != nil {
		writeError(w, http.StatusConflict, "cancellation_unavailable")
		return
	}
	respond(w, map[string]string{"status": "accepted"}, nil, http.StatusAccepted)
}

func (a taskRunAPI) respondPermission(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, taskID := scopeParams(r)
	if err := a.tasks.AuthorizeExecution(r.Context(), currentUserID(r), orgID, workspaceID, taskID, true); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	var body permissionResponseRequest
	if !decodeBody(w, r, &body) {
		return
	}
	if body.RequestID == "" || body.SessionID == "" || body.OptionID == "" {
		writeError(w, http.StatusBadRequest, "invalid_permission")
		return
	}
	response := orchestrationmodel.PermissionResponse{TaskID: taskID, RunID: chi.URLParam(r, "runID"), SessionID: body.SessionID, RequestID: body.RequestID, OptionID: body.OptionID}
	if err := a.launches.RespondTaskPermission(r.Context(), orgID, workspaceID, taskID, response.RunID, response); err != nil {
		writeError(w, http.StatusConflict, "permission_unavailable")
		return
	}
	respond(w, map[string]string{"status": "accepted"}, nil, http.StatusAccepted)
}

func scopeParams(r *http.Request) (string, string, string) {
	return chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "taskID")
}

func (a taskRunAPI) resumeQueue(w http.ResponseWriter, r *http.Request) { a.queue(w, r, false) }

func (a taskRunAPI) discardQueue(w http.ResponseWriter, r *http.Request) { a.queue(w, r, true) }

func (a taskRunAPI) queue(w http.ResponseWriter, r *http.Request, discard bool) {
	orgID, workspaceID, taskID := scopeParams(r)
	if err := a.tasks.AuthorizeExecution(r.Context(), currentUserID(r), orgID, workspaceID, taskID, true); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	if err := a.launches.TaskQueue(r.Context(), orgID, workspaceID, taskID, chi.URLParam(r, "runID"), discard); err != nil {
		writeError(w, http.StatusConflict, "queue_unavailable")
		return
	}
	respond(w, map[string]string{"status": "accepted"}, nil, http.StatusOK)
}
