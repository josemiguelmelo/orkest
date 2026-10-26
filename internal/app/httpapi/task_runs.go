package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/josemiguelmelo/orkest/internal/task"
)

type taskRunAPI struct {
	tasks    *task.Service
	launches *orchestration.LaunchService
}

func (a taskRunAPI) routes(r chi.Router) {
	const prefix = "/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/runs"
	r.Route(prefix, func(r chi.Router) {
		r.Get("/", a.list)
		r.Get("/{runID}/events", a.events)
		r.Post("/{runID}/prompts", a.prompt)
		r.Post("/{runID}/cancel", a.cancel)
		r.Post("/{runID}/permissions/respond", a.respondPermission)
	})
}

type taskRunView struct {
	RunID         string                     `json:"runId"`
	TaskID        string                     `json:"taskId"`
	Status        orchestration.LaunchStatus `json:"status"`
	FailureStage  string                     `json:"failureStage,omitempty"`
	FailureReason string                     `json:"failureReason,omitempty"`
	LastSequence  uint64                     `json:"lastSequence"`
	UpdatedAt     string                     `json:"updatedAt"`
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
	views := make([]taskRunView, 0, len(items))
	for _, item := range items {
		views = append(views, runView(item))
	}
	respond(w, views, nil, http.StatusOK)
}

func (a taskRunAPI) events(w http.ResponseWriter, r *http.Request) {
	orgID, workspaceID, taskID := scopeParams(r)
	if _, err := a.tasks.Get(r.Context(), currentUserID(r), orgID, workspaceID, taskID); err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	after, err := strconv.ParseUint(r.URL.Query().Get("afterSequence"), 10, 64)
	if (err != nil && r.URL.Query().Get("afterSequence") != "") || after > uint64(1<<63-1) {
		writeError(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	events, err := a.launches.TaskEvents(r.Context(), orgID, workspaceID, taskID, chi.URLParam(r, "runID"), after)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found")
		return
	}
	respond(w, events, nil, http.StatusOK)
}

type promptRequest struct {
	CommandID string `json:"commandId"`
	Text      string `json:"text"`
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
	if body.Text == "" || len([]byte(body.Text)) > agentctl.MaxPromptBytes || strings.TrimSpace(body.CommandID) == "" || len(body.CommandID) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_prompt")
		return
	}
	if err := a.launches.SendTaskPrompt(r.Context(), orgID, workspaceID, taskID, chi.URLParam(r, "runID"), body.CommandID, body.Text); err != nil {
		writeError(w, http.StatusConflict, "prompt_unavailable")
		return
	}
	respond(w, map[string]string{"commandId": body.CommandID}, nil, http.StatusAccepted)
}

type cancelRequest struct {
	CommandID string `json:"commandId"`
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

type permissionResponseRequest struct {
	RequestID string `json:"requestId"`
	SessionID string `json:"sessionId"`
	OptionID  string `json:"optionId"`
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
	response := agentctl.PermissionResponse{TaskID: taskID, RunID: chi.URLParam(r, "runID"), SessionID: body.SessionID, RequestID: body.RequestID, OptionID: body.OptionID}
	if err := a.launches.RespondTaskPermission(r.Context(), orgID, workspaceID, taskID, response.RunID, response); err != nil {
		writeError(w, http.StatusConflict, "permission_unavailable")
		return
	}
	respond(w, map[string]string{"status": "accepted"}, nil, http.StatusAccepted)
}

func scopeParams(r *http.Request) (string, string, string) {
	return chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "taskID")
}

func runView(item orchestration.Launch) taskRunView {
	return taskRunView{RunID: item.RunID, TaskID: item.TaskID, Status: item.Status, FailureStage: item.FailureStage, FailureReason: item.FailureReason, LastSequence: item.LastSequence, UpdatedAt: item.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")}
}
