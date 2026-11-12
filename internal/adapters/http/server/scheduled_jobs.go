package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	"github.com/josemiguelmelo/orkest/internal/server/scheduledjob"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
)

type scheduledJobAPI struct {
	service    *scheduledjob.ScheduledJobService
	launches   *orchestration.LaunchService
	dispatcher *orchestration.JobDispatcher
}

func (a scheduledJobAPI) routes(r chi.Router) {
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/scheduled-jobs", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.save)
		r.Post("/preview", a.preview)
		r.Route("/{jobID}", func(r chi.Router) {
			r.Get("/", a.get)
			r.Put("/", a.save)
			r.Get("/runs", a.runs)
			r.Post("/runs", a.runNow)
			r.Route("/runs/{occurrenceID}", func(r chi.Router) {
				r.Get("/", a.run)
				r.Get("/events", a.events)
				r.Post("/cancel", a.cancel)
				r.Post("/permissions/respond", a.permission)
				r.Post("/questions/{questionID}/respond", a.question)
			})
		})
	})
}

func (a scheduledJobAPI) list(w http.ResponseWriter, r *http.Request) {
	org, workspace, _ := scopeParams(r)
	items, err := a.service.List(r.Context(), currentUserID(r), org, workspace)
	respond(w, items, err, http.StatusOK)
}

func (a scheduledJobAPI) get(w http.ResponseWriter, r *http.Request) {
	org, workspace, _ := scopeParams(r)
	item, err := a.service.Get(r.Context(), currentUserID(r), org, workspace, chi.URLParam(r, "jobID"))
	respond(w, item, err, http.StatusOK)
}

func (a scheduledJobAPI) save(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CommandID string         `json:"commandId"`
		Revision  int64          `json:"revision"`
		Input     jobmodel.Input `json:"input"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	id := chi.URLParam(r, "jobID")
	if id == "" {
		if body.Revision != 0 {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		id = body.CommandID
	} else if body.Revision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	org, workspace, _ := scopeParams(r)
	item, err := a.service.Save(r.Context(), currentUserID(r), org, workspace, id, body.Revision, body.Input)
	respond(w, item, err, http.StatusOK)
}

func (a scheduledJobAPI) preview(w http.ResponseWriter, r *http.Request) {
	org, workspace, _ := scopeParams(r)
	if err := a.service.Authorize(r.Context(), currentUserID(r), org, workspace, false); err != nil {
		respondError(w, err)
		return
	}
	var body jobmodel.Schedule
	if !decodeBody(w, r, &body) {
		return
	}
	times := []time.Time{}
	after := time.Now()
	for range 5 {
		next, err := scheduledjob.Next(body, after)
		if err != nil {
			respondError(w, err)
			return
		}
		times = append(times, next)
		after = next
	}
	respond(w, times, nil, http.StatusOK)
}

func (a scheduledJobAPI) runs(w http.ResponseWriter, r *http.Request) {
	org, workspace, _ := scopeParams(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, e := strconv.Atoi(raw)
		if e != nil {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		offset = parsed
	}
	items, err := a.service.Runs(r.Context(), currentUserID(r), org, workspace, chi.URLParam(r, "jobID"), limit, offset)
	respond(w, items, err, http.StatusOK)
}

func (a scheduledJobAPI) runNow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CommandID string `json:"commandId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	org, workspace, _ := scopeParams(r)
	item, err := a.service.RunNow(r.Context(), currentUserID(r), org, workspace, chi.URLParam(r, "jobID"), body.CommandID)
	respond(w, item, err, http.StatusAccepted)
}

func (a scheduledJobAPI) scopedRun(w http.ResponseWriter, r *http.Request, write bool) (jobmodel.Run, bool) {
	org, workspace, _ := scopeParams(r)
	item, err := a.service.Run(r.Context(), currentUserID(r), org, workspace, chi.URLParam(r, "jobID"), chi.URLParam(r, "occurrenceID"), write)
	if err != nil {
		respondError(w, err)
		return jobmodel.Run{}, false
	}
	return item, true
}

func (a scheduledJobAPI) run(w http.ResponseWriter, r *http.Request) {
	if item, ok := a.scopedRun(w, r, false); ok {
		respond(w, item, nil, http.StatusOK)
	}
}

func (a scheduledJobAPI) events(w http.ResponseWriter, r *http.Request) {
	item, ok := a.scopedRun(w, r, false)
	if !ok {
		return
	}
	after, err := parseEventCursor(r.URL.Query().Get("afterSequence"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	if item.State == "pending" || item.State == "skipped" || item.State == "missed" || item.State == "needs_attention" || item.State == "delivery_uncertain" {
		respond(w, []orchestrationmodel.Event{}, nil, http.StatusOK)
		return
	}
	events, err := a.launches.TaskEvents(r.Context(), item.OrganizationID, item.WorkspaceID, item.ID, item.RunID, after)
	if errors.Is(err, orchestration.ErrTaskRunNotFound) {
		events, err = []orchestrationmodel.Event{}, nil
	}
	respond(w, events, err, http.StatusOK)
}

func (a scheduledJobAPI) cancel(w http.ResponseWriter, r *http.Request) {
	item, ok := a.scopedRun(w, r, true)
	if !ok {
		return
	}
	done, err := a.service.CancelPending(r.Context(), currentUserID(r), item.OrganizationID, item.WorkspaceID, item.JobID, item.ID)
	if err == nil && !done && (item.State == "starting" || item.State == "running") {
		item.CancelRequested = true
		err = a.dispatcher.StopJob(r.Context(), item)
	}
	respond(w, nil, err, http.StatusAccepted)
}

func (a scheduledJobAPI) permission(w http.ResponseWriter, r *http.Request) {
	item, ok := a.scopedRun(w, r, true)
	if !ok {
		return
	}
	var body permissionResponseRequest
	if !decodeBody(w, r, &body) {
		return
	}
	err := a.launches.RespondTaskPermission(r.Context(), item.OrganizationID, item.WorkspaceID, item.ID, item.RunID, orchestrationmodel.PermissionResponse{TaskID: item.ID, RunID: item.RunID, RequestID: body.RequestID, SessionID: body.SessionID, OptionID: body.OptionID})
	respond(w, nil, err, http.StatusAccepted)
}

func (a scheduledJobAPI) question(w http.ResponseWriter, r *http.Request) {
	item, ok := a.scopedRun(w, r, true)
	if !ok {
		return
	}
	response, ok := readQuestionResponse(w, r, item.ID)
	if !ok {
		return
	}
	response.RunID = item.RunID
	if err := a.launches.RespondTaskQuestion(r.Context(), item.OrganizationID, item.WorkspaceID, item.ID, item.RunID, response); err != nil {
		writeQuestionError(w, err)
		return
	}
	respond(w, map[string]string{"status": "answer_recorded"}, nil, http.StatusOK)
}
