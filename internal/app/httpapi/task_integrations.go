package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/taskintegration"
)

type taskIntegrationAPI struct {
	service *taskintegration.TaskIntegrationService
}

func (a taskIntegrationAPI) routes(r chi.Router) {
	r.Get("/task-integration-providers", a.providers)
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/task-integrations", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.create)
		r.Post("/validate", a.validate)
		r.Route("/{integrationID}", func(r chi.Router) {
			r.Put("/", a.update)
			r.Delete("/", a.delete)
			r.Post("/sync", a.syncNow)
		})
	})
}

func (a taskIntegrationAPI) providers(w http.ResponseWriter, r *http.Request) {
	respond(w, a.service.Providers(), nil, http.StatusOK)
}

func (a taskIntegrationAPI) list(w http.ResponseWriter, r *http.Request) {
	v, err := a.service.List(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, v, err, http.StatusOK)
}

func (a taskIntegrationAPI) create(w http.ResponseWriter, r *http.Request) {
	var body taskintegration.TaskIntegrationInput
	if !decodeBody(w, r, &body) {
		return
	}
	v, err := a.service.Create(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), body)
	respond(w, v, err, http.StatusCreated)
}

func (a taskIntegrationAPI) update(w http.ResponseWriter, r *http.Request) {
	var body taskintegration.TaskIntegrationInput
	if !decodeBody(w, r, &body) {
		return
	}
	v, err := a.service.Update(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "integrationID"), body)
	respond(w, v, err, http.StatusOK)
}

func (a taskIntegrationAPI) delete(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.service.Delete(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "integrationID")), http.StatusNoContent)
}

func (a taskIntegrationAPI) validate(w http.ResponseWriter, r *http.Request) {
	var body taskintegration.TaskIntegrationInput
	if !decodeBody(w, r, &body) {
		return
	}
	err := a.service.Validate(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), body)
	if err != nil {
		respond(w, nil, err, http.StatusOK)
		return
	}
	respond(w, map[string]string{"status": "ok"}, nil, http.StatusOK)
}

func (a taskIntegrationAPI) syncNow(w http.ResponseWriter, r *http.Request) {
	err := a.service.SyncNow(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "integrationID"))
	respond(w, nil, err, http.StatusNoContent)
}
