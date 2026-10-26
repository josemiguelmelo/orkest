package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/task"
)

type taskAPI struct{ tasks *task.Service }

func (a taskAPI) routes(r chi.Router) {
	const prefix = "/organizations/{orgID}/workspaces/{workspaceID}"
	r.Route(prefix+"/tasks", func(r chi.Router) {
		r.Get("/", a.listTasks)
		r.Post("/", a.createTask)
		r.Route("/{taskID}", func(r chi.Router) {
			r.Get("/", a.getTask)
			r.Put("/", a.updateTask)
			r.Get("/comments", a.listComments)
			r.Post("/comments", a.addComment)
		})
	})
	r.Route(prefix+"/components", func(r chi.Router) {
		r.Get("/", a.listComponents)
		r.Post("/", a.createComponent)
		r.Put("/{componentID}", a.updateComponent)
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
	var in task.TaskInput
	if !decodeBody(w, r, &in) {
		return
	}
	v, e := a.tasks.Create(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), in)
	respond(w, v, e, http.StatusCreated)
}

func (a taskAPI) updateTask(w http.ResponseWriter, r *http.Request) {
	var in task.TaskInput
	if !decodeBody(w, r, &in) {
		return
	}
	v, e := a.tasks.Update(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "taskID"), in)
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

func (a taskAPI) listComponents(w http.ResponseWriter, r *http.Request) {
	v, e := a.tasks.Components(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, v, e, http.StatusOK)
}

func (a taskAPI) createComponent(w http.ResponseWriter, r *http.Request) {
	var in task.ComponentInput
	if !decodeBody(w, r, &in) {
		return
	}
	v, e := a.tasks.CreateComponent(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), in)
	respond(w, v, e, http.StatusCreated)
}

func (a taskAPI) updateComponent(w http.ResponseWriter, r *http.Request) {
	var in task.ComponentInput
	if !decodeBody(w, r, &in) {
		return
	}
	v, e := a.tasks.UpdateComponent(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "componentID"), in)
	respond(w, v, e, http.StatusOK)
}
