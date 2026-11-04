package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
)

type workflowAPI struct {
	service *workflow.WorkflowConfigurationService
}

func (a workflowAPI) routes(r chi.Router) {
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/workflows", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.create)
		r.Route("/{workflowID}", func(r chi.Router) {
			r.Get("/", a.get)
			r.Put("/", a.save)
			r.Delete("/", a.delete)
		})
	})
}

func (a workflowAPI) list(w http.ResponseWriter, r *http.Request) {
	d, err := a.service.List(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, d, err, http.StatusOK)
}

func (a workflowAPI) create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	d, err := a.service.Create(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), input.Name)
	respond(w, d, err, http.StatusCreated)
}

func (a workflowAPI) get(w http.ResponseWriter, r *http.Request) {
	d, err := a.service.Get(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "workflowID"))
	respond(w, d, err, http.StatusOK)
}

func (a workflowAPI) save(w http.ResponseWriter, r *http.Request) {
	var d workflow.Definition
	if !decodeBody(w, r, &d) {
		return
	}
	saved, err := a.service.Save(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "workflowID"), d)
	respond(w, saved, err, http.StatusOK)
}

func (a workflowAPI) delete(w http.ResponseWriter, r *http.Request) {
	err := a.service.Delete(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "workflowID"))
	respond(w, nil, err, http.StatusNoContent)
}
