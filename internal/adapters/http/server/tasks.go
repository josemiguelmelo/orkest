package httpapi

import (
	"net/http"

	workflowautomation "github.com/josemiguelmelo/orkest/internal/server/workflowautomation"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/task"
)

type taskAPI struct {
	tasks      *task.TaskService
	automation *workflowautomation.WorkflowAutomationService
}

func (a taskAPI) routes(r chi.Router) {
	const prefix = "/organizations/{orgID}/workspaces/{workspaceID}"
	r.Route(prefix+"/tasks", func(r chi.Router) {
		r.Get("/", a.listTasks)
		r.Post("/", a.createTask)
		r.Route("/{taskID}", func(r chi.Router) {
			r.Get("/", a.getTask)
			r.Put("/", a.updateTask)
			r.Post("/transitions", a.transition)
			r.Get("/comments", a.listComments)
			r.Post("/comments", a.addComment)
		})
	})
}

func (a taskAPI) listTasks(w http.ResponseWriter, r *http.Request) {
	v, e := a.tasks.List(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, v, e, http.StatusOK)
}

func (a taskAPI) getTask(w http.ResponseWriter, r *http.Request) {
	v, e := a.tasks.Get(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "taskID"))
	respond(w, v, e, http.StatusOK)
}

func (a taskAPI) createTask(w http.ResponseWriter, r *http.Request) {
	var in taskMutationRequest
	if !decodeBody(w, r, &in) {
		return
	}
	v, e := a.tasks.Create(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), in.domain())
	respond(w, v, e, http.StatusCreated)
}

func (a taskAPI) updateTask(w http.ResponseWriter, r *http.Request) {
	var in taskMutationRequest
	if !decodeBody(w, r, &in) {
		return
	}
	v, e := a.tasks.Update(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "taskID"), in.domain())
	respond(w, v, e, http.StatusOK)
}

func (a taskAPI) listComments(w http.ResponseWriter, r *http.Request) {
	v, e := a.tasks.Comments(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "taskID"))
	respond(w, v, e, http.StatusOK)
}

func (a taskAPI) addComment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body string `json:"body"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	v, e := a.tasks.AddComment(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "taskID"), in.Body)
	respond(w, v, e, http.StatusCreated)
}

func (a taskAPI) transition(w http.ResponseWriter, r *http.Request) {
	var in workflowTransitionRequest
	if !decodeBody(w, r, &in) {
		return
	}
	value, err := a.automation.Transition(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "taskID"), in.domain())
	respond(w, value, err, http.StatusOK)
}
